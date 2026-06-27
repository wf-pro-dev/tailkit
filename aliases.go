package tailkit

import "github.com/wf-pro-dev/tailkit/client"

type (
	CPU                 = client.CPU
	Event[T any]        = client.Event[T]
	Host                = client.Host
	JournalEntry        = client.JournalEntry
	JobUpdate           = client.JobUpdate
	LogLine             = client.LogLine
	Memory              = client.Memory
	Metrics             = client.Metrics
	Peer                = client.Peer
	Port                = client.Port
	PortUpdate          = client.PortUpdate
	Process             = client.Process
	Service             = client.Service
	ServiceCapabilities = client.ServiceCapabilities
	ServiceStatus       = client.ServiceStatus
	Tailkitd            = client.Tailkitd
)

var (
	ErrConflict     = client.ErrConflict
	ErrUnauthorized = client.ErrUnauthorized
)
