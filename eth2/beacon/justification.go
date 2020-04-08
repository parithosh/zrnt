package beacon

import (
	"fmt"
	. "github.com/protolambda/ztyp/view"
)

func (state *BeaconStateView) ProcessEpochJustification(epc *EpochsContext, process *EpochProcess) error {
	previousEpoch := process.PrevEpoch
	currentEpoch := process.CurrEpoch

	// skip if genesis.
	if currentEpoch <= GENESIS_EPOCH+1 {
		return nil
	}

	prJustCh, err := state.PreviousJustifiedCheckpoint()
	if err != nil {
		return err
	}
	oldPreviousJustified, err := prJustCh.Raw()
	if err != nil {
		return err
	}
	cuJustCh, err := state.CurrentJustifiedCheckpoint()
	if err != nil {
		return err
	}
	oldCurrentJustified, err := cuJustCh.Raw()
	if err != nil {
		return err
	}

	bitsView, err := state.JustificationBits()
	if err != nil {
		return err
	}
	bits, err := bitsView.Raw()
	if err != nil {
		return err
	}

	// Rotate (a copy of) current into previous
	if err := prJustCh.Set(&oldCurrentJustified); err != nil {
		return err
	}

	bits.NextEpoch()

	// stake = effective balances of active validators
	// Get the total stake of the epoch attesters
	prevEpochStake := process.PrevEpochStake
	currEpochStake := process.CurrEpochStake

	// Get the total current stake
	totalStake := process.TotalActiveStake

	var newJustifiedCheckpoint *Checkpoint
	// > Justification
	if prevEpochStake.TargetStake*3 >= totalStake*2 {
		root, err := state.GetBlockRoot(previousEpoch)
		if err != nil {
			return err
		}
		newJustifiedCheckpoint = &Checkpoint{
			Epoch: previousEpoch,
			Root:  root,
		}
		bits[0] |= 1 << 1
	}
	if currEpochStake.TargetStake*3 >= totalStake*2 {
		root, err := state.GetBlockRoot(currentEpoch)
		if err != nil {
			return err
		}
		newJustifiedCheckpoint = &Checkpoint{
			Epoch: currentEpoch,
			Root:  root,
		}
		bits[0] |= 1 << 0
	}
	if newJustifiedCheckpoint != nil {
		if err := cuJustCh.Set(newJustifiedCheckpoint); err != nil {
			return err
		}
	}

	// > Finalization
	var toFinalize *Checkpoint
	// The 2nd/3rd/4th most recent epochs are all justified, the 2nd using the 4th as source
	if justified := bits.IsJustified(1, 2, 3); justified && oldPreviousJustified.Epoch+3 == currentEpoch {
		toFinalize = &oldPreviousJustified
	}
	// The 2nd/3rd most recent epochs are both justified, the 2nd using the 3rd as source
	if justified := bits.IsJustified(1, 2); justified && oldPreviousJustified.Epoch+2 == currentEpoch {
		toFinalize = &oldPreviousJustified
	}
	// The 1st/2nd/3rd most recent epochs are all justified, the 1st using the 3rd as source
	if justified := bits.IsJustified(0, 1, 2); justified && oldCurrentJustified.Epoch+2 == currentEpoch {
		toFinalize = &oldCurrentJustified
	}
	// The 1st/2nd most recent epochs are both justified, the 1st using the 2nd as source
	if justified := bits.IsJustified(0, 1); justified && oldCurrentJustified.Epoch+1 == currentEpoch {
		toFinalize = &oldCurrentJustified
	}
	if toFinalize != nil {
		finCh, err := state.FinalizedCheckpoint()
		if err != nil {
			return err
		}
		if err := finCh.Set(toFinalize); err != nil {
			return err
		}
	}
	if err := bitsView.Set(bits); err != nil {
		return err
	}
	return nil
}

type JustificationBits [1]byte

func (jb *JustificationBits) BitLen() uint64 {
	return JUSTIFICATION_BITS_LENGTH
}

// Prepare bitfield for next epoch by shifting previous bits (truncating to bitfield length)
func (jb *JustificationBits) NextEpoch() {
	// shift and mask
	jb[0] = (jb[0] << 1) & 0x0f
}

func (jb *JustificationBits) IsJustified(epochsAgo ...Epoch) bool {
	for _, t := range epochsAgo {
		if jb[0]&(1<<t) == 0 {
			return false
		}
	}
	return true
}

var JustificationBitsType = BitVectorType(JUSTIFICATION_BITS_LENGTH)

type JustificationBitsView struct {
	*BitVectorView
}

func (v *JustificationBitsView) Raw() (JustificationBits, error) {
	b, err := v.SubtreeView.GetNode(0)
	if err != nil {
		return JustificationBits{}, err
	}
	r, ok := b.(*Root)
	if !ok {
		return JustificationBits{}, fmt.Errorf("justification bitvector bottom node is not a root, cannot get bits")
	}
	return JustificationBits{r[0]}, nil
}

func (v *JustificationBitsView) Set(bits JustificationBits) error {
	root := Root{0: bits[0]}
	return v.SetBacking(&root)
}

func AsJustificationBits(v View, err error) (*JustificationBitsView, error) {
	c, err := AsBitVector(v, err)
	return &JustificationBitsView{c}, err
}