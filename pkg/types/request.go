package types

type AllocationType string

const (
	AllocationTypeMPS       AllocationType = "MPS"
	AllocationTypeFastPod   AllocationType = "FASTPOD"
	AllocationTypeExclusive AllocationType = "EXCLUSIVE"
	AllocationTypeNone      AllocationType = "NONE"
)

func GetAllocationType(allocationType string) AllocationType {
	switch allocationType {
	case "MPS":
		return AllocationTypeMPS
	case "FastPod":
		return AllocationTypeFastPod
	case "Exclusive":
		return AllocationTypeExclusive
	default:
		return AllocationTypeNone
	}
}

func (a AllocationType) String() string {
	switch a {
	case AllocationTypeMPS:
		return "MPS"
	case AllocationTypeFastPod:
		return "FastPod"
	case AllocationTypeExclusive:
		return "Exclusive"
	default:
		return "Unknown"
	}
}

func (AllocationType) Values() []string {
	return []string{
		string(AllocationTypeMPS),
		string(AllocationTypeFastPod),
		string(AllocationTypeExclusive),
	}
}
