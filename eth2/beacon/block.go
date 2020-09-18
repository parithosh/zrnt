package beacon

import (
	"github.com/protolambda/ztyp/tree"
	. "github.com/protolambda/ztyp/view"
)

type SignedBeaconBlock struct {
	Message   BeaconBlock
	Signature BLSSignature
}

func (block *SignedBeaconBlock) SignedHeader() *SignedBeaconBlockHeader {
	return &SignedBeaconBlockHeader{
		Message:   *block.Message.Header(),
		Signature: block.Signature,
	}
}

type BeaconBlock struct {
	Slot          Slot
	ProposerIndex ValidatorIndex
	ParentRoot    Root
	StateRoot     Root
	Body          BeaconBlockBody
}

func (b *BeaconBlock) HashTreeRoot(hFn tree.HashFn) Root {
	return hFn.HashTreeRoot(b.Slot, b.ProposerIndex, b.ParentRoot, b.StateRoot, b.Body)
}

func (c *Phase0Config) BeaconBlock() *ContainerTypeDef {
	return ContainerType("BeaconBlock", []FieldDef{
		{"slot", SlotType},
		{"proposer_index", ValidatorIndexType},
		{"parent_root", RootType},
		{"state_root", RootType},
		{"body", c.BeaconBlockBody()},
	})
}

func (c *Phase0Config) SignedBeaconBlock() *ContainerTypeDef {
	return ContainerType("SignedBeaconBlock", []FieldDef{
		{"message", c.BeaconBlock()},
		{"signature", BLSSignatureType},
	})
}

func (block *BeaconBlock) Header() *BeaconBlockHeader {
	return &BeaconBlockHeader{
		Slot:          block.Slot,
		ProposerIndex: block.ProposerIndex,
		ParentRoot:    block.ParentRoot,
		StateRoot:     block.StateRoot,
		BodyRoot:      block.Body.HashTreeRoot(),
	}
}

type BeaconBlockBody struct {
	RandaoReveal BLSSignature
	Eth1Data     Eth1Data // Eth1 data vote
	Graffiti     Root     // Arbitrary data

	ProposerSlashings ProposerSlashings
	AttesterSlashings AttesterSlashings
	Attestations      Attestations
	Deposits          Deposits
	VoluntaryExits    VoluntaryExits
}

func (b *BeaconBlockBody) HashTreeRoot(hFn tree.HashFn) Root {
	return hFn.HashTreeRoot(
		b.RandaoReveal, &b.Eth1Data,
		b.Graffiti, &b.ProposerSlashings,
		&b.AttesterSlashings, &b.Attestations,
		&b.Deposits, &b.VoluntaryExits,
	)
}

func (c *Phase0Config) BeaconBlockBody() *ContainerTypeDef {
	return ContainerType("BeaconBlockBody", []FieldDef{
		{"randao_reveal", BLSSignatureType},
		{"eth1_data", c.Eth1Data()}, // Eth1 data vote
		{"graffiti", Bytes32Type},   // Arbitrary data
		// Operations
		{"proposer_slashings", c.BlockProposerSlashings()},
		{"attester_slashings", c.BlockAttesterSlashings()},
		{"attestations", c.BlockAttestations()},
		{"deposits", c.BlockDeposits()},
		{"voluntary_exits", c.BlockVoluntaryExits()},
	})
}
