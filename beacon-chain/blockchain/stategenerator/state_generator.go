package stategenerator

import (
	"context"
	"fmt"

	b "github.com/prysmaticlabs/prysm/beacon-chain/core/blocks"
	"github.com/prysmaticlabs/prysm/beacon-chain/core/state"
	"github.com/prysmaticlabs/prysm/beacon-chain/db"
	pb "github.com/prysmaticlabs/prysm/proto/beacon/p2p/v1"
	"github.com/prysmaticlabs/prysm/shared/bytesutil"
	"github.com/sirupsen/logrus"
)

// GenerateStateFromSlot generates state from the last finalized epoch till the specified block.
func GenerateStateFromBlock(ctx context.Context, db *db.BeaconDB, block *pb.BeaconBlock) (*pb.BeaconState, error) {
	fState, err := db.FinalizedState()
	if err != nil {
		return nil, err
	}

	if fState.Slot > block.Slot {
		return nil, fmt.Errorf(
			"requested slot %d < current slot %d in the finalized beacon state",
			fState.Slot,
			block,
		)
	}

	root, err := b.BlockRoot(fState, fState.Slot-1)
	if err != nil {
		return nil, fmt.Errorf("unable to get block root %v", err)
	}

	finalizedBlockRoot := bytesutil.ToBytes32(root)
	ancestorSet, err := lookUpFromFinalizedBlock(ctx, db, block, finalizedBlockRoot)
	if err != nil {
		return nil, fmt.Errorf("unable to look up block ancestors %v", err)
	}

	logrus.Errorf("Start: Current slot %d and Finalized Epoch %d", fState.Slot, fState.FinalizedEpoch)

	for i := len(ancestorSet); i > 0; i-- {
		block := ancestorSet[i-1]

		if block.Slot <= fState.Slot {
			continue
		}
		// Running state transitions for skipped slots.
		for block.Slot != fState.Slot+1 {
			fState, err = state.ExecuteStateTransition(
				ctx,
				fState,
				nil,
				bytesutil.ToBytes32(block.ParentRootHash32),
				true, /* sig verify */
			)
			if err != nil {
				return nil, fmt.Errorf("could not execute state transition %v", err)
			}
		}

		fState, err = state.ExecuteStateTransition(
			ctx,
			fState,
			block,
			bytesutil.ToBytes32(block.ParentRootHash32),
			true, /* sig verify */
		)
		if err != nil {
			return nil, fmt.Errorf("could not execute state transition %v", err)
		}
	}

	logrus.Errorf("End: Current slot %d and Finalized Epoch %d", fState.Slot, fState.FinalizedEpoch)

	return fState, nil
}

func lookUpFromFinalizedBlock(ctx context.Context, db *db.BeaconDB, block *pb.BeaconBlock,
	finalizedBlockRoot [32]byte) ([]*pb.BeaconBlock, error) {

	blockAncestors := make([]*pb.BeaconBlock, 0)
	blockAncestors = append(blockAncestors, block)

	parentRoot := bytesutil.ToBytes32(block.ParentRootHash32)
	// looking up ancestors, till it gets the last finalized block.
	for parentRoot != finalizedBlockRoot {
		retblock, err := db.Block(parentRoot)
		if err != nil {
			return nil, err
		}
		blockAncestors = append(blockAncestors, retblock)
		parentRoot = bytesutil.ToBytes32(retblock.ParentRootHash32)
	}

	return blockAncestors, nil
}
