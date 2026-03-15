
package pitcher

import (
	"fmt"
	"log/slog"
	"strings"

	homerun "github.com/stuttgart-things/homerun-library/v3"
)

// MultiPitcher sends messages to multiple pitcher backends.
type MultiPitcher struct {
	Pitchers []Pitcher
}

// Pitch sends the message to all backends and returns the first successful
// objectID/streamID. It logs errors for individual failures but only returns
// an error if ALL backends fail.
func (p *MultiPitcher) Pitch(msg homerun.Message) (string, string, error) {
	var (
		firstObjectID string
		firstStreamID string
		errs          []string
		succeeded     bool
	)

	for i, backend := range p.Pitchers {
		objectID, streamID, err := backend.Pitch(msg)
		if err != nil {
			slog.Error("multi pitcher: backend failed",
				"index", i,
				"error", err,
			)
			errs = append(errs, fmt.Sprintf("backend %d: %v", i, err))
			continue
		}

		slog.Debug("multi pitcher: backend succeeded",
			"index", i,
			"objectID", objectID,
			"streamID", streamID,
		)

		if !succeeded {
			firstObjectID = objectID
			firstStreamID = streamID
			succeeded = true
		}
	}

	if !succeeded {
		return "", "", fmt.Errorf("multi pitcher: all backends failed: %s", strings.Join(errs, "; "))
	}

	return firstObjectID, firstStreamID, nil
}
