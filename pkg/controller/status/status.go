package status

const (
	DisabledStatus = "disabled"
	UploadStatus   = "upload"
	DownloadStatus = "download"
	ErrorStatus    = "error"
)

type controllerStatus struct {
	statusMap map[string]statusMessage
}

type statusMessage struct {
	reason string
	message string
}

func newControllerStatus() *controllerStatus {
	return &controllerStatus{
		statusMap: map[string]statusMessage{},
	}
}

func (c *controllerStatus) setStatus(id, reason, message string) {
	entries := make(map[string]statusMessage)
	for k, v := range c.statusMap {
		entries[k] = v
	}

	existing, ok := c.statusMap[id]
	if !ok || existing.reason != reason || existing.message != message {
		entries[id] = statusMessage{
			reason: reason,
			message: message,
		}
	}

	c.statusMap = entries
}

func (c *controllerStatus) getStatus(id string) *statusMessage {
	s, ok := c.statusMap[id]
	if !ok {
		return nil
	}

	return &s
}

func (c *controllerStatus) hasStatus(id string) bool {
	_, ok := c.statusMap[id]
	return ok
}
