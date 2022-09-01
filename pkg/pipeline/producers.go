package pipeline

import "context"

func Producer[T any](ctx context.Context, urls []T) (<-chan T, error){
	outChannel := make(chan T)
	go func() {
		defer close(outChannel)
		for _, url := range urls {
			outChannel <- url
		}
	}()
	return outChannel, nil
}




