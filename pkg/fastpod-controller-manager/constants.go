package fastpodcontrollermanager

const (
	// SuccessSynced is used as part of the Event 'reason' when a FaSTPod is synced
	SuccessSynced = "Synced"
	// ErrResourceExists is used as part of the Event 'reason' when a FaSTPod fails
	// to sync due to a Deployment of the same name already existing.
	ErrResourceExists = "ErrResourceExists"

	// MessageResourceExists is the message used for Events when a resource
	// fails to sync due to a Deployment already existing
	MessageResourceExists = "Resource %q already exists and is not managed by FaSTPod"
	// MessageResourceSynced is the message used for an Event fired when a FaSTPod
	// is synced successfully
	MessageResourceSynced = "FaSTPod synced successfully"

	fastpodKind    = "FaSTPod"
	FastGShareWarm = "fast-gshare/warmpool"

	FaSTPodLibraryDir  = "/fastpod/library"
	SchedulerIpFile    = FaSTPodLibraryDir + "/schedulerIP.txt"
	GPUClientPortStart = 56001
	GPUSchedPortStart  = 52001

	ErrValueError             = "ErrValueError"
	SlowStartInitialBatchSize = 1
)
