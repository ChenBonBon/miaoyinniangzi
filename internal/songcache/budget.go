package songcache

import "context"

type playbackReadKey struct{}

// At most three speculative reads can occupy the four upstream slots, leaving
// one for an uncached read requested by the currently playing native decoder.
type budget struct{ all, background chan struct{} }

func newBudget() *budget {
	return &budget{all: make(chan struct{}, 4), background: make(chan struct{}, 3)}
}
func (b *budget) acquire(ctx context.Context, foreground bool) (func(), error) {
	if !foreground {
		select {
		case b.background <- struct{}{}:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	select {
	case b.all <- struct{}{}:
		return func() {
			<-b.all
			if !foreground {
				<-b.background
			}
		}, nil
	case <-ctx.Done():
		if !foreground {
			<-b.background
		}
		return nil, ctx.Err()
	}
}
