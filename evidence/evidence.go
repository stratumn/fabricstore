package evidence

import (
	"encoding/json"

	"github.com/stratumn/sdk/cs"
)

const (
	// ProofBackend used in Evidence to deserialize proof
	ProofBackend = "fabric"
)

// New returns a fabric evidence
func New(TransactionID string, timestamp uint64) (*cs.Evidence, error) {
	return &cs.Evidence{
		State:    cs.CompleteEvidence,
		Backend:  ProofBackend,
		Provider: ProofBackend,
		Proof: &FabricProof{
			Timestamp:     timestamp,
			TransactionID: TransactionID,
		},
	}, nil
}

// FabricProof implements stratumn/sdk/cs/Proof
type FabricProof struct {
	Timestamp     uint64 `json:"timestamp"`
	TransactionID string `json:"transaction_id"`
}

// Time returns the timestamp from the block header
func (p *FabricProof) Time() uint64 {
	return p.Timestamp
}

// FullProof returns a JSON formatted proof
func (p *FabricProof) FullProof() []byte {
	bytes, err := json.MarshalIndent(p, "", "   ")
	if err != nil {
		return nil
	}
	return bytes
}

// Verify returns true if the proof of a given linkHash is correct
func (p *FabricProof) Verify(interface{}) bool {
	return true
}

// init needs to define a way to deserialize a FabricProof
func init() {
	cs.DeserializeMethods[ProofBackend] = func(rawProof json.RawMessage) (cs.Proof, error) {
		p := FabricProof{}
		if err := json.Unmarshal(rawProof, &p); err != nil {
			return nil, err
		}
		return &p, nil
	}
}
