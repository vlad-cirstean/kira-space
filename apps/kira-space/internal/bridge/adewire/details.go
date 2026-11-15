package adewire

// DiskChange is one file under a task folder whose content differs from what the app accepted.
// State is new, edited or missing; Diff is the unified text diff for a text file.
type DiskChange struct {
	Path    string `json:"path"`
	Kind    string `json:"kind"`
	State   string `json:"state"`
	Diff    string `json:"diff"`
	Pending bool   `json:"pending"`
	Problem string `json:"problem"`
}

// DiskChangesArgs names the task whose folder to compare.
type DiskChangesArgs struct {
	TaskID string `json:"taskId"`
}

// DiskResolveArgs accepts or discards changes; empty Paths means every change.
type DiskResolveArgs struct {
	TaskID string   `json:"taskId"`
	Paths  []string `json:"paths"`
}

// DetailsFolder is the absolute path of a task's folder.
type DetailsFolder struct {
	Path string `json:"path"`
}
