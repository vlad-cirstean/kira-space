Kira Space's Api module reads this folder. Each collection is a `<id>.collection.yaml` file and each environment a `<id>.environment.yaml` file.

Substitution:

- `{{name}}` in a URL, a header name or value, a body text field, a gRPC target, message or metadata is replaced when the request is sent.
- Environment variables override collection variables. `{{$name}}` is a dynamic value. An unknown name stays literal.

Secrets:

- Write a secret variable as `secret: ${secret:<name>}`. The file never holds the value.
- Set the value in Kira, under Api, Environments, Secrets. An unset secret fails the send and names the secret.

Ids:

- The file name and each item `id` are stable ids. Renaming one deletes the history of the old id. Change `name` freely.
- The order of `items` in the file is the order in the tree.

Kira's Save rewrites only the saved request's node. Comments elsewhere stay.
