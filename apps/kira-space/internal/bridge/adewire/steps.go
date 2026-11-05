package adewire

// StepEntry is one file of the step library (<home>/steps/<id>.yaml).
type StepEntry struct {
	FileName string         `json:"fileName"`
	Path     string         `json:"path"`
	Step     *WorkflowStep  `json:"step"`  // last valid version; nil if never valid
	Error    *WorkflowError `json:"error"` // current file's error
	// UsedBy lists the ids of the workflows whose steps reference this one.
	UsedBy []string `json:"usedBy"`
	Hash   string   `json:"hash"`
}

type StepYaml struct {
	FileName string `json:"fileName"`
	Path     string `json:"path"`
	Yaml     string `json:"yaml"`
	// Hash identifies Yaml; SaveStepYamlArgs.BaseHash sends it back.
	Hash string `json:"hash"`
}

type StepValidation struct {
	Step  *WorkflowStep  `json:"step"`
	Error *WorkflowError `json:"error"`
}

type SaveStepArgs struct {
	FileName string       `json:"fileName"`
	Step     WorkflowStep `json:"step"`
	// BaseHash is the StepEntry.Hash the draft was built on; "" skips the check.
	BaseHash string `json:"baseHash"`
}

type SaveStepYamlArgs struct {
	FileName string `json:"fileName"`
	Yaml     string `json:"yaml"`
	// BaseHash is the StepYaml.Hash the text was edited from; "" skips the check.
	BaseHash string `json:"baseHash"`
}

type ValidateStepYamlArgs struct {
	Yaml string `json:"yaml"`
}

// NewStepArgs creates a library step. From set, the step is a copy of that inline step.
type NewStepArgs struct {
	Name string        `json:"name"`
	Kind StepKind      `json:"kind"`
	From *WorkflowStep `json:"from"`
}

type DeleteStepArgs struct {
	FileName string `json:"fileName"`
}
