package appupdate

// App names the installable app: ScriptArg is scripts/install.sh's --app= value, Name is what the
// checker stamps into its User-Agent and what the installer passes through to the script and shows
// in the dialog.
type App struct{ Name, ScriptArg string }

var Space = App{Name: "Kira Space", ScriptArg: "space"}
