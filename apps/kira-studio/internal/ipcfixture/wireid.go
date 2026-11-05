package ipcfixture

import (
	"crypto/sha1" //nolint:gosec // stable label-to-UUID mapping, not security
	"encoding/hex"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapterhost"
	"github.com/kirathecat/kira-studio/internal/ids"
)

// WireOpID maps a readable fixture label ("be-read-orders") to a stable UUID-shaped op id, because
// the host refuses a non-UUID op id. A real UUID passes through. The recorded fixtures keep the label.
func WireOpID(label string) string {
	if label == "" || ids.IsUUID(label) {
		return label
	}
	sum := sha1.Sum([]byte(label)) //nolint:gosec // see above
	h := hex.EncodeToString(sum[:])
	return h[:8] + "-" + h[8:12] + "-7" + h[13:16] + "-8" + h[17:20] + "-" + h[20:32]
}

// WireRead returns req with its op id mapped by WireOpID.
func WireRead(req adapterhost.ReadRequestWire) adapterhost.ReadRequestWire {
	req.OpID = WireOpID(req.OpID)
	return req
}

// WireCount returns req with its op id mapped by WireOpID.
func WireCount(req adapterhost.CountRequestWire) adapterhost.CountRequestWire {
	req.OpID = WireOpID(req.OpID)
	return req
}

// WireExecute returns req with its op id mapped by WireOpID.
func WireExecute(req adapterhost.ExecuteRequestWire) adapterhost.ExecuteRequestWire {
	req.OpID = WireOpID(req.OpID)
	return req
}

// WireMutate returns req with its op id mapped by WireOpID.
func WireMutate(req adapterhost.MutateRequestWire) adapterhost.MutateRequestWire {
	req.OpID = WireOpID(req.OpID)
	return req
}
