# Chat log: <project slug>

Verbatim and append-only. Copy user lines as typed. Leave every line above the last one alone: a correction becomes a new line further down. [../AGENTS.md](../AGENTS.md#the-chat-log) states the format rules.

| Nick     | Who                                              |
|----------|--------------------------------------------------|
| `black`  | the user, the local part of the git author email |
| `claude` | the agent                                        |

```
[YYYY-MM-DD HH:MM] <black> the opening ask, as typed
--- topic: discovery
[YYYY-MM-DD HH:MM] <claude> the seven discovery questions, as asked
[YYYY-MM-DD HH:MM] <black> the answers, as typed
[YYYY-MM-DD HH:MM] <claude> README.md written, confirm or amend
[YYYY-MM-DD HH:MM] <black> confirmed
--- topic: search
* claude searches "<the exact query>"
* claude reads <the exact path or URL>
[YYYY-MM-DD HH:MM] <claude> what the source said, citing R1
--- note: <an out-of-band fact, a pasted link, a session resuming>
```

## Log

<!-- Append below this line. Delete the example block above once the first real
entries land. -->
