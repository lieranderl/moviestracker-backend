package firestorestore

import (
	"context"
	"fmt"
	"maps"
	"net"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
	pb "cloud.google.com/go/firestore/apiv1/firestorepb"
	"github.com/lieranderl/moviestracker-backend/internal/catalog"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestUpsertRejectsMovieWithoutStableDocumentID(t *testing.T) {
	repository := Repository{}

	_, err := repository.Upsert(context.Background(), catalog.CollectionLatest, []catalog.Movie{{Title: "Missing ID"}})

	require.ErrorContains(t, err, "movie id is required")
}

func TestUpsertPersistsMovieContract(t *testing.T) {
	repository, _ := testRepository(t)
	found := time.Date(2026, 9, 5, 1, 2, 3, 0, time.UTC)
	movie := catalog.Movie{
		ID: "9737", LastTimeFound: found, Title: "Bad Boys", OriginalTitle: "Bad Boys",
		GenreIDs: []int32{28, 35}, Year: "1995", ReleaseDate: "1995-04-07", VoteAverage: "6.8", VoteCount: "123",
		BackdropPath: "/backdrop.jpg", PosterPath: "/poster.jpg", SearchName: "Not stored", SearchAliases: []string{"Not stored"},
	}

	written, err := repository.Upsert(context.Background(), catalog.CollectionLatest, []catalog.Movie{movie})
	require.NoError(t, err)
	require.Equal(t, 1, written)
	snapshot, err := repository.Client.Collection(string(catalog.CollectionLatest)).Doc("9737").Get(context.Background())
	require.NoError(t, err)

	require.Equal(t, map[string]any{
		"id": "9737", "lasttimefound": found, "title": "Bad Boys", "original_title": "Bad Boys",
		"genre_ids": []any{int64(28), int64(35)}, "Year": "1995", "release_date": "1995-04-07",
		"vote_average": "6.8", "vote_count": "123", "backdrop_path": "/backdrop.jpg", "poster_path": "/poster.jpg",
	}, snapshot.Data())
}

func TestUpsertSkipsDocumentsThatWouldNotChange(t *testing.T) {
	found := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	movie := catalog.Movie{ID: "9737", Title: "Bad Boys", GenreIDs: []int32{28, 35}, LastTimeFound: found, Year: "1995"}

	tests := map[string]struct {
		movie   catalog.Movie
		written int
	}{
		"identical":             {movie: movie, written: 0},
		"older discovery":       {movie: withFound(movie, found.Add(-24*time.Hour)), written: 0},
		"newer discovery":       {movie: withFound(movie, found.Add(24*time.Hour)), written: 1},
		"changed metadata":      {movie: withTitle(movie, "Bad Boys (Remastered)"), written: 1},
		"genres become missing": {movie: withGenres(movie, nil), written: 1},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			repository, database := testRepository(t)
			upsert(t, repository, movie)
			database.mu.Lock()
			commits := database.commits
			database.mu.Unlock()

			written, err := repository.Upsert(context.Background(), catalog.CollectionLatest, []catalog.Movie{test.movie})

			require.NoError(t, err)
			require.Equal(t, test.written, written)
			database.mu.Lock()
			defer database.mu.Unlock()
			require.Equal(t, commits+test.written, database.commits, "an unchanged movie opens no transaction")
		})
	}
}

func TestUpsertReadsExistingDocumentsInBatches(t *testing.T) {
	repository, database := testRepository(t)
	movies := make([]catalog.Movie, 2*readBatchSize+1)
	for i := range movies {
		movies[i] = catalog.Movie{ID: fmt.Sprint(i), LastTimeFound: time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)}
	}
	written, err := repository.Upsert(context.Background(), catalog.CollectionLatest, movies)
	require.NoError(t, err)
	require.Equal(t, len(movies), written)

	written, err = repository.Upsert(context.Background(), catalog.CollectionLatest, movies)

	require.NoError(t, err)
	require.Zero(t, written)
	database.mu.Lock()
	defer database.mu.Unlock()
	require.Equal(t, 6, database.plainReads, "three batched reads per run")
	require.Equal(t, len(movies), database.writes, "the second run wrote nothing")
}

func TestSameDocumentIsExactAboutShape(t *testing.T) {
	found := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	data := documentData(catalog.Movie{ID: "1", GenreIDs: []int32{28}, LastTimeFound: found})
	stored := func(change func(map[string]any)) map[string]any {
		document := maps.Clone(data)
		document["genre_ids"] = []any{int64(28)}
		document["lasttimefound"] = found.In(time.FixedZone("CET", 3600))
		change(document)
		return document
	}

	require.True(t, sameDocument(stored(func(map[string]any) {}), data))
	require.False(t, sameDocument(stored(func(d map[string]any) { delete(d, "poster_path") }), data), "missing field")
	require.False(t, sameDocument(stored(func(d map[string]any) { d["extra"] = "x" }), data), "extra field")
	require.False(t, sameDocument(stored(func(d map[string]any) { d["genre_ids"] = nil }), data), "null against a list")
	require.False(t, sameDocument(stored(func(d map[string]any) { d["genre_ids"] = []any{int64(35)} }), data), "different genre")
	require.False(t, sameDocument(stored(func(d map[string]any) { d["genre_ids"] = []any{} }), data), "empty against a list")
	require.False(t, sameDocument(stored(func(d map[string]any) { d["title"] = int64(1) }), data), "different type")

	empty := documentData(catalog.Movie{ID: "1", GenreIDs: []int32{}, LastTimeFound: found})
	require.False(t, sameDocument(stored(func(d map[string]any) { d["genre_ids"] = nil }), empty), "null against an empty list")
	require.True(t, sameDocument(stored(func(d map[string]any) { d["genre_ids"] = []any{} }), empty))
}

func upsert(t *testing.T, repository *Repository, movie catalog.Movie) {
	t.Helper()
	_, err := repository.Upsert(context.Background(), catalog.CollectionLatest, []catalog.Movie{movie})
	require.NoError(t, err)
}

func withFound(movie catalog.Movie, found time.Time) catalog.Movie {
	movie.LastTimeFound = found
	return movie
}

func withTitle(movie catalog.Movie, title string) catalog.Movie {
	movie.Title = title
	return movie
}

func withGenres(movie catalog.Movie, genres []int32) catalog.Movie {
	movie.GenreIDs = genres
	return movie
}

func TestDeduplicateMoviesKeepsNewestTMDBDocument(t *testing.T) {
	old := catalog.Movie{ID: "1", LastTimeFound: time.Unix(1, 0), Title: "old"}
	newest := catalog.Movie{ID: "1", LastTimeFound: time.Unix(2, 0), Title: "new"}

	got := deduplicateMovies([]catalog.Movie{old, newest})

	require.Equal(t, []catalog.Movie{newest}, got)
}

func TestUpsertNeverMovesStoredDiscoveryTimeBackwards(t *testing.T) {
	repository, database := testRepository(t)
	newer := catalog.Movie{ID: "9737", Title: "Newer", LastTimeFound: time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)}
	older := catalog.Movie{ID: "9737", Title: "Older", LastTimeFound: time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)}

	upsert(t, repository, newer)
	upsert(t, repository, older)

	document := database.document("projects/test-project/databases/moviestracker/documents/latesttorrentsmovies/9737")
	require.NotNil(t, document)
	require.Equal(t, newer.LastTimeFound, document.Fields["lasttimefound"].GetTimestampValue().AsTime())
	require.Equal(t, "Older", document.Fields["title"].GetStringValue(), "metadata can update without regressing discovery time")
}

func TestUpsertRetriesWhenConcurrentWriterFindsMovieLater(t *testing.T) {
	repository, database := testRepository(t)
	ctx := context.Background()
	movie := catalog.Movie{ID: "9737", Title: "Initial", LastTimeFound: time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)}
	upsert(t, repository, movie)
	name := "projects/test-project/databases/moviestracker/documents/latesttorrentsmovies/9737"
	concurrent := database.document(name)
	latest := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	concurrent.Fields["lasttimefound"] = &pb.Value{ValueType: &pb.Value_TimestampValue{TimestampValue: timestamppb.New(latest)}}
	database.mu.Lock()
	database.conflictingDocument = concurrent
	database.mu.Unlock()

	movie.LastTimeFound = time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	written, err := repository.Upsert(ctx, catalog.CollectionLatest, []catalog.Movie{movie})
	require.NoError(t, err)

	require.Equal(t, latest, database.document(name).Fields["lasttimefound"].GetTimestampValue().AsTime())
	require.Zero(t, written, "the retry found the concurrent writer had already stored a newer discovery")
	database.mu.Lock()
	defer database.mu.Unlock()
	require.Equal(t, 3, database.commits, "initial write, conflicted write, and the retry's empty commit")
	require.Equal(t, 1, database.writes)
}

func TestUpsertHonorsCallerCancellation(t *testing.T) {
	repository, _ := testRepository(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := repository.Upsert(ctx, catalog.CollectionLatest, []catalog.Movie{{ID: "9737"}})

	require.Error(t, err)
	require.Equal(t, codes.Canceled, status.Code(err))
}

// This fake lives at the Firestore RPC boundary; repository and SDK code are real.
type firestoreDatabase struct {
	pb.UnimplementedFirestoreServer
	mu                  sync.Mutex
	documents           map[string]*pb.Document
	conflictingDocument *pb.Document
	commits             int
	// plainReads counts BatchGetDocuments calls made outside a transaction.
	plainReads int
	writes     int
}

func testRepository(t testing.TB) (*Repository, *firestoreDatabase) {
	t.Helper()
	// Explicitly disable emulator routing: this test always uses an in-memory RPC connection.
	t.Setenv("FIRESTORE_EMULATOR_HOST", "")
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	database := &firestoreDatabase{documents: make(map[string]*pb.Document)}
	pb.RegisterFirestoreServer(server, database)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	connection, err := grpc.NewClient("passthrough:///firestore-test",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	require.NoError(t, err)
	t.Cleanup(func() { _ = connection.Close() })
	client, err := firestore.NewClientWithDatabase(context.Background(), "test-project", "moviestracker", option.WithGRPCConn(connection))
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	return New(client), database
}

func (d *firestoreDatabase) document(name string) *pb.Document {
	d.mu.Lock()
	defer d.mu.Unlock()
	if document := d.documents[name]; document != nil {
		return proto.Clone(document).(*pb.Document)
	}
	return nil
}

func (d *firestoreDatabase) BeginTransaction(context.Context, *pb.BeginTransactionRequest) (*pb.BeginTransactionResponse, error) {
	return &pb.BeginTransactionResponse{Transaction: []byte("transaction")}, nil
}

func (d *firestoreDatabase) BatchGetDocuments(request *pb.BatchGetDocumentsRequest, stream pb.Firestore_BatchGetDocumentsServer) error {
	if request.GetTransaction() == nil {
		d.mu.Lock()
		d.plainReads++
		d.mu.Unlock()
	}
	for _, name := range request.Documents {
		response := &pb.BatchGetDocumentsResponse{ReadTime: timestamppb.Now()}
		if document := d.document(name); document != nil {
			response.Result = &pb.BatchGetDocumentsResponse_Found{Found: document}
		} else {
			response.Result = &pb.BatchGetDocumentsResponse_Missing{Missing: name}
		}
		if err := stream.Send(response); err != nil {
			return err
		}
	}
	return nil
}

func (d *firestoreDatabase) apply(writes []*pb.Write) []*pb.WriteResult {
	d.mu.Lock()
	defer d.mu.Unlock()
	results := make([]*pb.WriteResult, len(writes))
	for i, write := range writes {
		document := proto.Clone(write.GetUpdate()).(*pb.Document)
		document.CreateTime = timestamppb.Now()
		document.UpdateTime = document.CreateTime
		d.documents[document.Name] = document
		d.writes++
		results[i] = &pb.WriteResult{UpdateTime: document.UpdateTime}
	}
	return results
}

func (d *firestoreDatabase) Commit(_ context.Context, request *pb.CommitRequest) (*pb.CommitResponse, error) {
	d.mu.Lock()
	d.commits++
	if document := d.conflictingDocument; document != nil {
		d.documents[document.Name] = document
		d.conflictingDocument = nil
		d.mu.Unlock()
		return nil, status.Error(codes.Aborted, "concurrent document update")
	}
	d.mu.Unlock()
	return &pb.CommitResponse{CommitTime: timestamppb.Now(), WriteResults: d.apply(request.Writes)}, nil
}

func (d *firestoreDatabase) Rollback(context.Context, *pb.RollbackRequest) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

// BenchmarkUpsertCatalog measures the steady state of a scheduled run: every
// movie is already stored with identical data.
func BenchmarkUpsertCatalog(b *testing.B) {
	repository, database := testRepository(b)
	movies := make([]catalog.Movie, 20)
	for i := range movies {
		movies[i] = catalog.Movie{ID: fmt.Sprint(i), LastTimeFound: time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)}
	}
	if _, err := repository.Upsert(context.Background(), catalog.CollectionLatest, movies); err != nil {
		b.Fatal(err)
	}
	database.mu.Lock()
	database.commits, database.writes = 0, 0
	database.mu.Unlock()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := repository.Upsert(context.Background(), catalog.CollectionLatest, movies); err != nil {
			b.Fatal(err)
		}
	}
	database.mu.Lock()
	b.ReportMetric(float64(database.commits)/float64(b.N), "commits/op")
	b.ReportMetric(float64(database.writes)/float64(b.N), "writes/op")
	database.mu.Unlock()
}
