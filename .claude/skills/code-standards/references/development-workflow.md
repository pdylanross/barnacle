# Development Workflow

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

## Commit Message Style

Follow conventional commits where appropriate:
* `feat:` - New feature
* `fix:` - Bug fix
* `doc:` or `docs:` - Documentation
* `refactor:` - Code refactoring
* `test:` - Adding or updating tests
* `chore:` - Maintenance tasks

## GitHub CLI (`gh`) Reference

This project uses the `gh` CLI for all GitHub interactions. Authentication is handled via `gh auth login` (already configured for this repo).

### Creating a Branch and Pushing

```bash
# Start from an up-to-date main
git checkout main
git pull origin main
git checkout -b feat/<short-name>

# Stage and commit changes
git add <files>
git commit -m "feat: description of changes"

# Push and set upstream tracking (-u only needed on first push)
git push -u origin feat/<short-name>
```

### Creating a Pull Request

Use `gh pr create` to open a PR from the current branch:

```bash
# Basic PR creation (will prompt for title and body if omitted)
gh pr create --base main --title "feat: description" --body "Summary of changes"

# Use commit messages to auto-fill title and body
gh pr create --base main --fill

# Use first commit for title, all commits for body
gh pr create --base main --fill-first --fill-verbose

# Create a draft PR
gh pr create --base main --title "feat: WIP description" --draft

# Use a heredoc for multi-line body
gh pr create --base main --title "feat: description" --body "$(cat <<'EOF'
## Summary
- Change 1
- Change 2

## Test Plan
- [ ] Unit tests pass
- [ ] E2E tests pass
EOF
)"
```

### Checking PR Status

```bash
# View the current branch's PR details
gh pr view

# Check CI status for the current branch's PR
gh pr checks

# Watch CI checks until they complete (polls every 10s)
gh pr checks --watch

# Watch with a custom interval (seconds)
gh pr checks --watch --interval 30

# Only show required checks
gh pr checks --required

# List your open PRs
gh pr list --author "@me"
```

### Merging a Pull Request

Always use **squash merge** for this project:

```bash
# Squash merge the current branch's PR and delete the branch
gh pr merge --squash --delete-branch

# Squash merge with a custom commit message
gh pr merge --squash --delete-branch \
  --subject "feat: description (#123)" \
  --body "Detailed description of the squashed changes"

# Enable auto-merge (merges automatically once CI passes and review is approved)
gh pr merge --squash --delete-branch --auto

# Merge a specific PR by number
gh pr merge 123 --squash --delete-branch
```

### Full Workflow Example

```bash
# 1. Create branch
git checkout main && git pull origin main
git checkout -b feat/blob-ttl

# 2. Do the work, commit
git add internal/registry/cache/disk/cache.go
git commit -m "feat: add TTL support for disk blob cache"

# 3. Push
git push -u origin feat/blob-ttl

# 4. Create PR
gh pr create --base main \
  --title "feat: add TTL support for disk blob cache" \
  --body "Adds configurable TTL eviction to the disk-based blob cache."

# 5. Wait for CI (watch checks in terminal)
gh pr checks --watch

# 6. After approval, squash merge and clean up
gh pr merge --squash --delete-branch

# 7. Return to main
git checkout main && git pull origin main
```

### Auto-Merge Workflow

When CI is still running or review is pending, enable auto-merge so the PR merges as soon as all requirements are met:

```bash
# Create PR and immediately enable auto-merge
gh pr create --base main \
  --title "feat: description" \
  --body "Summary of changes"

gh pr merge --squash --delete-branch --auto
```

To disable auto-merge if plans change:

```bash
gh pr merge --disable-auto
```