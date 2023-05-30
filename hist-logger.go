package forwardproxy

type HistLogger interface {
	LogAccepted(fqdn string)
	LogBlocked(fqdn string)
}
