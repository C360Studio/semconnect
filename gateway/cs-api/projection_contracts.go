package csapi

import (
	"context"
	"fmt"

	"github.com/c360studio/semconnect/parser/sensorml"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/ownership"
	"github.com/c360studio/semstreams/pkg/projection"
	"github.com/c360studio/semstreams/vocabulary"
)

const systemProjectionOwner = "cs-api-systems"

func systemProjectionContract(systemIDPrefix string) projection.Contract {
	systemPattern := systemIDPrefix + ".*"
	return projection.Contract{
		Name:          "cs-api.system.foreign-edges",
		MessageType:   systemProjectionMessageType.Key(),
		EntityPattern: systemPattern,
		ForeignEdges: []projection.ForeignEdge{{
			Predicate:     sensorml.PredIsHostedBy,
			Mode:          ownership.EdgeNoBirthStub,
			TargetPattern: systemPattern,
		}},
	}
}

func (c *Component) bindProjectionContracts(ctx context.Context) error {
	client, ok := c.nats.(*natsclient.Client)
	if !ok {
		c.logger.Debug("skipping cs-api projection contract binding: requester is not a semstreams NATS client")
		return nil
	}
	reg, err := ownership.EnsureBuckets(ctx, client, c.logger, vocabulary.InverseResolver)
	if err != nil {
		return fmt.Errorf("ensure ownership buckets: %w", err)
	}
	// beta.113 (ADR-055/056 flip): Bind now returns the owner's OwnerToken. We
	// discard it — this is an FE-ONLY owner (the contract derives a single
	// ForeignEdgeClaim and zero OwnerClaims, see the test), and an FE-only claim
	// is compaction-exempt (epoch.go), so it needs no heartbeat / OwnerToken
	// lease. (A static owner that derived a real OwnerClaim would have to
	// BindAndHeartbeat to keep its OWNER_PRESENCE key alive.)
	if _, err := projection.Bind(ctx, reg, systemProjectionOwner,
		systemProjectionContract(c.cfg.SystemIDPrefix)); err != nil {
		return fmt.Errorf("bind System projection contract: %w", err)
	}
	c.logger.Info("bound cs-api System projection contract",
		"owner", systemProjectionOwner,
		"producer", systemProjectionMessageType.Key(),
		"system_pattern", c.cfg.SystemIDPrefix+".*")
	return nil
}
