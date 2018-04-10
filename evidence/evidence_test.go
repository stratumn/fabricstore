package evidence

import (
	"flag"
	"fmt"
	"testing"

	"github.com/stratumn/go-indigocore/cs"
)

var (
	integration = flag.Bool("integration", false, "Run integration tests")
)

func TestNew(t *testing.T) {
	if _, err := New("1", uint64(1)); err != nil {
		fmt.Println("Could not generate evidence")
		t.FailNow()
	}
}

func TestProof(t *testing.T) {
	proof := FabricProof{
		Timestamp:     uint64(1),
		TransactionID: "1",
	}
	if proof.Time() != uint64(1) {
		t.FailNow()
	}
	proofBytes := proof.FullProof()
	if proofBytes == nil {
		t.FailNow()
	}
	if proof.Verify(nil) != true {
		t.FailNow()
	}
	_, err := cs.DeserializeMethods[ProofBackend](proofBytes)
	if err != nil {
		t.FailNow()
	}
}
