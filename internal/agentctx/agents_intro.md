`context.md` is the agent context: Markdown text that every agent Kira starts gets as extra system prompt, whatever the provider. Interactive sessions, Take over, resume, headless steps and smart scripts all get it.

To change it, edit `context.md` in this folder with any editor, or use the Context page in the Agents module of Kira Space. An empty or missing file means no context.

An edit applies to agents started after the save: new sessions, headless steps and smart scripts. A running session keeps the context it started with. Some providers also keep the recorded context of a resumed conversation until it compacts.

Every agent reads this file. Never put secrets in it.
