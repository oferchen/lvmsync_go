package common

// MergeHandshake selects preferred parameters from local and peer handshakes.
//
// Fields already set on local take precedence. When a field such as Transport,
// Compress, or Digest is unset, MergeHandshake chooses the first common element
// between the corresponding capability lists.
func MergeHandshake(local, peer Handshake) Handshake {
	if local.Transport == "" {
		local.Transport = SelectBest(local.Transports, append(peer.Transports, peer.Transport))
	}
	if local.Compress == "" {
		local.Compress = SelectBest(local.Compressors, append(peer.Compressors, peer.Compress))
	}
	if local.Digest == "" {
		local.Digest = SelectBest(local.Digests, append(peer.Digests, peer.Digest))
	}
	return local
}
