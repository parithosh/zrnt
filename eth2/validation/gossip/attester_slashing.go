package gossip

import (
	"context"
	"errors"
	"fmt"
	"github.com/protolambda/zrnt/eth2/beacon"
)

// Check if all of the indices have been seen before, return true if so.
// May not be an index within valid range.
// It is up to the topic subscriber to mark indices as seen. Indices which are checked may not be valid,
// and should not be marked as seen because of just the check itself.
// It is recommended to regard any indices which were finalized as slashed, as seen.
type AttesterSlashableAllSeenFn func(indices []beacon.ValidatorIndex) bool

func (gv *GossipValidator) ValidateAttesterSlashing(ctx context.Context, attSl *beacon.AttesterSlashing, seenFn AttesterSlashableAllSeenFn) GossipValidatorResult {
	sa1 := &attSl.Attestation1
	sa2 := &attSl.Attestation2

	// [REJECT] All of the conditions within process_attester_slashing pass validation.
	// Part 1: just light checks, make sure the formatting is right, no signature checks yet.
	if !beacon.IsSlashableAttestationData(&sa1.Data, &sa2.Data) {
		return GossipValidatorResult{REJECT, errors.New("attester slashing has no valid reasoning")}
	}
	indices1, err := gv.Spec.ValidateIndexedAttestationIndicesSet(sa1)
	if err != nil {
		return GossipValidatorResult{REJECT, errors.New("attestation 1 of attester slashing cannot be verified")}
	}
	indices2, err := gv.Spec.ValidateIndexedAttestationIndicesSet(sa2)
	if err != nil {
		return GossipValidatorResult{REJECT, errors.New("attestation 2 of attester slashing cannot be verified")}
	}

	// [IGNORE] At least one index in the intersection of the attesting indices of each attestation has not yet been seen in any prior attester_slashing
	slashable := make(beacon.ValidatorSet, 0, len(indices1))
	indices1.ZigZagJoin(indices2, func(i beacon.ValidatorIndex) {
		slashable = append(slashable, i)
	}, nil)

	if seenFn(slashable) {
		return GossipValidatorResult{IGNORE, errors.New("no unseen slashable attester indices")}
	}

	_, epc, state, err := gv.HeadInfo(ctx)
	if err != nil {
		return GossipValidatorResult{IGNORE, err}
	}
	validators, err := state.Validators()
	if err != nil {
		return GossipValidatorResult{IGNORE, errors.New("no access to validators state data")}
	}
	// [REJECT] All of the conditions within process_attester_slashing pass validation.
	// Part 2: make sure validators are actually slashable
	err = slashable.Filter(func(index beacon.ValidatorIndex) (bool, error) {
		validator, err := validators.Validator(index)
		if err != nil {
			return false, err
		}
		// only retain the slashable indices
		return gv.Spec.IsSlashable(validator, epc.CurrentEpoch.Epoch)
	})
	if err != nil {
		return GossipValidatorResult{REJECT, fmt.Errorf("cannot access validator data: %v", err)}
	}
	if len(slashable) == 0 {
		return GossipValidatorResult{REJECT, errors.New("no slashable validators remain after checking against current head state")}
	}

	// [REJECT] All of the conditions within process_attester_slashing pass validation.
	// Part 3: signature checks
	if err := gv.Spec.ValidateIndexedAttestation(epc, state, sa1); err != nil {
		return GossipValidatorResult{REJECT, fmt.Errorf("attester slashing att 1 signature is invalid: %v", err)}
	}
	if err := gv.Spec.ValidateIndexedAttestation(epc, state, sa2); err != nil {
		return GossipValidatorResult{REJECT, fmt.Errorf("attester slashing att 2 signature is invalid: %v", err)}
	}

	return GossipValidatorResult{ACCEPT, nil}
}