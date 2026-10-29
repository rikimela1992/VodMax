package service

import (
	"context"

	"github.com/rikimela1992/vodmax/internal/store"
)

// Viewer is who is watching: the profile whose favourites and progress
// are used, and whether it is a kids profile (adult content is then never
// listed or opened, whatever the client asks).
type Viewer struct {
	ProfileID int64
	Kids      bool
}

type viewerKey struct{}

// WithViewer attaches the viewer of a request.
func WithViewer(ctx context.Context, v Viewer) context.Context {
	return context.WithValue(ctx, viewerKey{}, v)
}

// viewerOf returns the request's viewer; internal work (recorder,
// scheduled jobs) acts as the default profile.
func viewerOf(ctx context.Context) Viewer {
	if v, ok := ctx.Value(viewerKey{}).(Viewer); ok {
		return v
	}
	return Viewer{ProfileID: DefaultProfile}
}

// allowItem hides adult items from kids profiles, as if they did not
// exist.
func (s *Service) allowItem(ctx context.Context, itemID int64) error {
	if !viewerOf(ctx).Kids {
		return nil
	}
	adult, err := s.st.ItemAdult(ctx, itemID)
	if err != nil {
		return err
	}
	if adult {
		return store.ErrNotFound
	}
	return nil
}

func (s *Service) allowGroup(ctx context.Context, groupKey string) error {
	if !viewerOf(ctx).Kids {
		return nil
	}
	adult, err := s.st.GroupAdult(ctx, groupKey)
	if err != nil {
		return err
	}
	if adult {
		return store.ErrNotFound
	}
	return nil
}
