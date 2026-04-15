# Copilot Instructions — Haruki Cloud

## Git Commit Format

All commits must follow this format:

```
[Type] Short description starting with capital letter
```

### Types

| Type | Usage |
|------|-------|
| `[Feat]` | New feature or capability |
| `[Fix]` | Bug fix |
| `[Chore]` | Maintenance, refactoring, dependency updates, build changes |
| `[Docs]` | Documentation changes only |

### Rules

- The description after the type tag must **start with a capital letter**
- Keep the message short and imperative (e.g. "Add snapshot debug logging", not "Added snapshot debug logging")
- Do not end with a period

### Examples

```
[Feat] Add toolbox live snapshot provider
[Fix] Move user_snapshot config under pjsk_render
[Chore] Rename config file to haruki-cloud.yaml
[Docs] Update known-bugs.md with snapshot fix
```
