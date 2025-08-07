// common/common.go
package common //nolint:revive // package name is established API

// ProtocolVersion identifies the string exchanged during the initial handshake.
// It follows the format "lvmsync PROTO[<n>]" where <n> is the protocol version
// number both peers must support.
const ProtocolVersion = "lvmsync PROTO[3]"
