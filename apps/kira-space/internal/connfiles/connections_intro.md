Each file is one database connection: where it is, how Kira connects, what an AI client may do with it. The file name without `.yaml` is the connection id.

A password or a URI never goes in a file. Write `password: ${secret:name}` or `uri: ${secret:name}`, then set the value in Kira, in the connection's file dialog. The value stays sealed in the app.

Kira reads a saved file at once. A file with an error keeps its last good content running. Keys left out take their default.
