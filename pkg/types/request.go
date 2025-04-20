package types

type AllocationType string

const (
	AllocationTypeMPS     AllocationType = "MPS"
	AllocationTypeFastPod AllocationType = "FastPod"
	AllocationTypeMIG     AllocationType = "MIG"
	AllocationTypeUnknown AllocationType = "Unknown"
)

func GetAllocationType(allocationType string) AllocationType {
	switch allocationType {
	case "MPS":
		return AllocationTypeMPS
	case "FastPod":
		return AllocationTypeFastPod
	case "MIG":
		return AllocationTypeMIG
	default:
		return AllocationTypeUnknown
	}
}

func (a AllocationType) String() string {
	switch a {
	case AllocationTypeMPS:
		return "MPS"
	case AllocationTypeFastPod:
		return "FastPod"
	case AllocationTypeMIG:
		return "MIG"
	default:
		return "Unknown"
	}
}

func (AllocationType) Values() []string {
	return []string{
		string(AllocationTypeMPS),
		string(AllocationTypeFastPod),
		string(AllocationTypeMIG),
	}
}
