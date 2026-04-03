package policysync

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"

	"github.com/cordon-co/cordon-cli/cli/internal/api"
	"github.com/cordon-co/cordon-cli/cli/internal/apicontract"
	"github.com/cordon-co/cordon-cli/cli/internal/store"
)

// LookupPerimeter checks whether the given perimeter is registered remotely.
// Returns (remotePerimeterID, true, nil) when registered, ("", false, nil) when not found.
func LookupPerimeter(client *api.Client, perimeterID string) (string, bool, error) {
	var resp apicontract.PerimeterLookupResponse
	_, err := client.GetJSON(
		fmt.Sprintf("/api/v1/perimeters/lookup?perimeter_id=%s", url.QueryEscape(perimeterID)),
		&resp,
	)
	if err != nil {
		if errors.Is(err, api.ErrNotFound) {
			return "", false, nil
		}
		return "", false, err
	}
	return resp.PerimeterId, true, nil
}

// PullEvents pulls policy events after local max(server_seq) and appends them
// to the local policy database.
func PullEvents(policyDB *sql.DB, client *api.Client, perimeterID string) (int, error) {
	totalPulled := 0
	afterSeq, err := store.MaxServerSeq(policyDB)
	if err != nil {
		return 0, err
	}

	for {
		var pullResp apicontract.PolicyPullResponse
		_, err = client.GetJSON(
			fmt.Sprintf("/api/v1/perimeters/%s/policy/events?after_server_seq=%d&limit=1000", perimeterID, afterSeq),
			&pullResp,
		)
		if err != nil {
			return totalPulled, err
		}

		events := make([]store.PolicyEvent, 0, len(pullResp.Events))
		for _, e := range pullResp.Events {
			events = append(events, store.PolicyEvent{
				EventID:   e.EventId,
				EventType: e.EventType,
				Payload:   e.Payload,
				Actor:     e.Actor,
				Timestamp: e.Timestamp,
				ServerSeq: e.ServerSeq,
			})
		}

		if len(events) == 0 {
			break
		}
		if err := store.AppendRemoteEvents(policyDB, events); err != nil {
			return totalPulled, err
		}
		totalPulled += len(events)

		if !pullResp.HasMore {
			break
		}
		lastEvent := events[len(events)-1]
		if lastEvent.ServerSeq == nil {
			break
		}
		afterSeq = *lastEvent.ServerSeq
	}
	return totalPulled, nil
}
