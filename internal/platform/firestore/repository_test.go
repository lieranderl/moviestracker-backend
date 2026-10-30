package firestorestore

import (
	"context"
	"fmt"
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

	err := repository.Upsert(context.Background(), catalog.CollectionLatest, []catalog.Movie{{Title: "Missing ID"}})

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

	require.NoError(t, repository.Upsert(context.Background(), catalog.CollectionLatest, []catalog.Movie{movie}))
	snapshot, err := repository.Client.Collection(string(catalog.CollectionLatest)).Doc("9737").Get(context.Background())
	require.NoError(t, err)

	require.Equal(t, map[string]any{
		"id": "9737", "lasttimefound": found, "title": "Bad Boys", "original_title": "Bad Boys",
		"genre_ids": []any{int64(28), int64(35)}, "Year": "1995", "release_date": "1995-04-07",
		"vote_average": "6.8", "vote_count": "123", "backdrop_path": "/backdrop.jpg", "poster_path": "/poster.jpg",
	}, snapshot.Data())
}

func TestDeduplicateMoviesKeepsNewestTMDBDocument(t *testing.T) {
	old := catalog.Movie{ID: "1", LastTimeFound: time.Unix(1, 0), Title: "old"}
	newest := catalog.Movie{ID: "1", LastTimeFound: time.Unix(2, 0), Title: "new"}

	got := deduplicateMovies([]catalog.Movie{old, newest})

	require.Equal(t, []catalog.Movie{newest}, got)
}

func TestUpsertNeverMovesStoredDiscoveryTimeBackwards(t *testing.T) {
	repository, database := testRepository(t)
	ctx := context.Background()
	newer := catalog.Movie{ID: "9737", Title: "Newer", LastTimeFound: time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)}
	older := catalog.Movie{ID: "9737", Title: "Older", LastTimeFound: time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)}

	require.NoError(t, repository.Upsert(ctx, catalog.CollectionLatest, []catalog.Movie{newer}))
	require.NoError(t, repository.Upsert(ctx, catalog.CollectionLatest, []catalog.Movie{older}))

	document := database.document("projects/test-project/databases/moviestracker/documents/latesttorrentsmovies/9737")
	require.NotNil(t, document)
	require.Equal(t, newer.LastTimeFound, document.Fields["lasttimefound"].GetTimestampValue().AsTime())
	require.Equal(t, "Older", document.Fields["title"].GetStringValue(), "metadata can update without regressing discovery time")
}

func TestUpsertRetriesWhenConcurrentWriterFindsMovieLater(t *testing.T) {
	repository, database := testRepository(t)
	ctx := context.Background()
	movie := catalog.Movie{ID: "9737", Title: "Initial", LastTimeFound: time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)}
	require.NoError(t, repository.Upsert(ctx, catalog.CollectionLatest, []catalog.Movie{movie}))
	name := "projects/test-project/databases/moviestracker/documents/latesttorrentsmovies/9737"
	concurrent := database.document(name)
	latest := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	concurrent.Fields["lasttimefound"] = &pb.Value{ValueType: &pb.Value_TimestampValue{TimestampValue: timestamppb.New(latest)}}
	database.mu.Lock()
	database.conflictingDocument = concurrent
	database.mu.Unlock()

	movie.LastTimeFound = time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	require.NoError(t, repository.Upsert(ctx, catalog.CollectionLatest, []catalog.Movie{movie}))

	require.Equal(t, latest, database.document(name).Fields["lasttimefound"].GetTimestampValue().AsTime())
	database.mu.Lock()
	defer database.mu.Unlock()
	require.Equal(t, 3, database.commits, "initial write, conflicted write, and successful retry")
}

func TestUpsertHonorsCallerCancellation(t *testing.T) {
	repository, _ := testRepository(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := repository.Upsert(ctx, catalog.CollectionLatest, []catalog.Movie{{ID: "9737"}})

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

func BenchmarkUpsertCatalog(b *testing.B) {
	repository, database := testRepository(b)
	movies := make([]catalog.Movie, 20)
	for i := range movies {
		movies[i] = catalog.Movie{ID: fmt.Sprint(i), LastTimeFound: time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)}
	}
	b.ReportAllocs()
	for b.Loop() {
		if err := repository.Upsert(context.Background(), catalog.CollectionLatest, movies); err != nil {
			b.Fatal(err)
		}
	}
	database.mu.Lock()
	b.ReportMetric(float64(database.commits)/float64(b.N), "writes/op")
	database.mu.Unlock()
}
