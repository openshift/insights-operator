package status

type statusID string

const (
	DisabledStatus     = "disabled"
	UploadStatus       = "upload"
	DownloadStatus     = "download"
	ErrorStatus        = "error"
	RemoteConfigStatus = "remoteConfig"
)

type controllerStatus struct {
	statusMap map[statusID]statusMessage
}

type statusMessage struct {
	reason  string
	message string
}

func newControllerStatus() *controllerStatus {
	return &controllerStatus{
		statusMap: make(map[statusID]statusMessage),
	}
}

func (c *controllerStatus) setStatus(id statusID, reason, message string) {
	entries := make(map[statusID]statusMessage)
	for k, v := range c.statusMap {
		entries[k] = v
	}

	existing, ok := c.statusMap[id]
	if !ok || existing.reason != reason || existing.message != message {
		entries[id] = statusMessage{
			reason:  reason,
			message: message,
		}
	}

	c.statusMap = entries
}

func (c *controllerStatus) getStatus(id statusID) *statusMessage {
	s, ok := c.statusMap[id]
	if !ok {
		return nil
	}

	return &s
}

func (c *controllerStatus) hasStatus(id statusID) bool {
	_, ok := c.statusMap[id]
	return ok
}

func (c *controllerStatus) reset() {
	c.statusMap = make(map[statusID]statusMessage)
}

func (c *controllerStatus) isHealthy() bool {
	return !c.hasStatus(ErrorStatus)
}

func (c *controllerStatus) isDisabled() bool {
	return c.hasStatus(DisabledStatus)
}
