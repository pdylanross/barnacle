---
name: development-workflow
description: Branch naming conventions, PR workflow, and development process for this repository
---

## Branch Naming Conventions

When starting work, create a branch from `main` using the appropriate prefix:

* `feat/<short-name>` - For new features (e.g., `feat/blob-replication`)
* `bug/<description>` - For bug fixes (e.g., `bug/redis-connection-timeout`)
* `doc/<description>` - For documentation changes (e.g., `doc/api-examples`)

## Development Workflow

1. **Create branch** from `main` with the appropriate prefix
2. **Do the work** - implement the feature, fix, or documentation
3. **Commit and push** - commit changes with descriptive messages, push to remote
4. **Create PR** - open a pull request targeting `main`
5. **Wait for CI** - ensure all pipelines pass
6. **Get review** - wait for code review approval
7. **Squash merge** - merge to `main` using squash merge

## Git Commands Reference

```bash
# Start a new feature
git checkout main
git pull origin main
git checkout -b feat/<short-name>

# After work is complete
git add <files>
git commit -m "feat: description of changes"
git push -u origin feat/<short-name>

# Create PR via GitHub CLI
gh pr create --base main --title "feat: description" --body "..."
```

## Commit Message Style

Follow conventional commits where appropriate:
* `feat:` - New feature
* `fix:` - Bug fix
* `doc:` or `docs:` - Documentation
* `refactor:` - Code refactoring
* `test:` - Adding or updating tests
* `chore:` - Maintenance tasks