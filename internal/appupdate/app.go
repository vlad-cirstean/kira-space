package appupdate

// App names the installable app: Name is what the checker stamps into its User-Agent and what the
// installer shows in the dialog and uses for the log path.
type App struct{ Name string }

var Space = App{Name: "Kira Space"}
