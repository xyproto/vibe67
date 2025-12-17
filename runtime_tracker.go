package main

type RuntimeFeature string

const (
	FeatureCPUDetection RuntimeFeature = "cpu_detection"
	FeatureMetaArena    RuntimeFeature = "meta_arena"
	FeatureStringConcat RuntimeFeature = "string_concat"
	FeatureStringToCstr RuntimeFeature = "string_to_cstr"
	FeatureCstrToString RuntimeFeature = "cstr_to_string"
	FeatureStringSlice  RuntimeFeature = "string_slice"
	FeatureStringEq     RuntimeFeature = "string_eq"
	FeatureListConcat   RuntimeFeature = "list_concat"
	FeatureListRepeat   RuntimeFeature = "list_repeat"
	FeatureArenaCreate  RuntimeFeature = "arena_create"
	FeatureArenaAlloc   RuntimeFeature = "arena_alloc"
	FeaturePrintf       RuntimeFeature = "printf"
	FeaturePrintSyscall RuntimeFeature = "print_syscall"
	FeatureSIMD         RuntimeFeature = "simd"
	FeatureFMA          RuntimeFeature = "fma"
	FeatureAVX2         RuntimeFeature = "avx2"
	FeatureAVX512       RuntimeFeature = "avx512"
	FeaturePOPCNT       RuntimeFeature = "popcnt"
)

type RuntimeFeatures struct {
	features map[RuntimeFeature]bool
}

func NewRuntimeFeatures() *RuntimeFeatures {
	return &RuntimeFeatures{
		features: make(map[RuntimeFeature]bool),
	}
}

func (rf *RuntimeFeatures) Mark(feature RuntimeFeature) {
	rf.features[feature] = true
}

func (rf *RuntimeFeatures) Uses(feature RuntimeFeature) bool {
	return rf.features[feature]
}

func (rf *RuntimeFeatures) needsCPUDetection() bool {
	return rf.Uses(FeatureSIMD) || rf.Uses(FeatureFMA) || 
		rf.Uses(FeatureAVX2) || rf.Uses(FeatureAVX512) || rf.Uses(FeaturePOPCNT)
}

func (rf *RuntimeFeatures) needsStringToCstr() bool {
	return rf.Uses(FeatureStringToCstr) || rf.Uses(FeaturePrintf)
}

func (rf *RuntimeFeatures) needsArenaInit() bool {
	return rf.Uses(FeatureMetaArena) || rf.Uses(FeatureArenaCreate) || 
		rf.Uses(FeatureArenaAlloc) || rf.Uses(FeatureStringConcat) || 
		rf.Uses(FeatureListConcat) || rf.Uses(FeatureListRepeat)
}
