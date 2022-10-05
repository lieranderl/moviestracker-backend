package pipeline

import (
	"context"
	"log"
	"sync"

	"golang.org/x/sync/semaphore"
)

func Step[In any, Out any](
	ctx context.Context,
	inputChannel <-chan In,
	fn func(In) (Out, error),
	limit int64,
) (chan Out, chan error) {
	outputChannel := make(chan Out)
	errorChannel := make(chan error)

	// Use all CPU cores to maximize efficiency. We'll set the limit to 2 so you
	// can see the values being processed in batches of 2 at a time, in parallel
	// limit := int64(runtime.NumCPU())
	sem1 := semaphore.NewWeighted(limit)

	go func() {
		defer close(outputChannel)
		defer close(errorChannel)

		for s := range inputChannel {
			select {
			case <-ctx.Done():
			default:
			}

			if err := sem1.Acquire(ctx, 1); err != nil {
				log.Printf("Failed to acquire semaphore: %v", err)
				break
			}

			go func(s In) {
				defer sem1.Release(1)

				result, err := fn(s)
				if err != nil {
					errorChannel <- err
				} else {
					outputChannel <- result
				}
			}(s)
		}

		if err := sem1.Acquire(ctx, limit); err != nil {
			log.Printf("Failed to acquire semaphore: %v", err)
		}
	}()

	return outputChannel, errorChannel
}

func Merge[T any](ctx context.Context, cs ...<-chan T) <-chan T {
	var wg sync.WaitGroup
	out := make(chan T)

	output := func(c <-chan T) {
		defer wg.Done()
		for n := range c {
			select {
			case out <- n:
			case <-ctx.Done():
				return
			}
		}
	}

	wg.Add(len(cs))
	for _, c := range cs {
		go output(c)
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}
