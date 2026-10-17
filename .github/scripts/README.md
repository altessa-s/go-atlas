# GitHub Actions Scripts

Scripts used in GitHub Actions workflows for automated validation and checks.

## validate-commits.sh

Validates commit messages against [Conventional Commits](https://www.conventionalcommits.org/) format and checks scopes against
[`commit_scopes.txt`](../../commit_scopes.txt).

### Features

- ✅ Validates Conventional Commits format: `type[(scope)]: description` (the scope is optional)
- ✅ Checks commit types: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`, `ci`, `perf`, `build`
- ✅ Validates scopes, when present, against `commit_scopes.txt`
- ✅ Skips merge, revert, and auto-merge commits, and automated `build(deps)` / `chore(deps)` commits (dependabot)
- ✅ Works for both PR and push events
- ✅ Provides detailed error messages and examples

### Usage

**In GitHub Actions:**
```yaml
- name: Validate commits
  run: .github/scripts/validate-commits.sh
```

**Locally:**
```bash
# Check local commits not in origin/develop
.github/scripts/validate-commits.sh

# Simulate GitHub Actions environment
GITHUB_EVENT_NAME=push .github/scripts/validate-commits.sh
```

### Examples

**Valid commit messages:**
```
feat(data/mongo): add cursor pagination support
fix(transport/http): handle nil pointer in retry logic
docs: update installation instructions
test(core/retry): add benchmark for exponential backoff
chore(deps): update dependencies to latest versions
```

**Invalid commit messages:**
```
Add new feature                    # Missing type
feat(invalid): add feature         # Invalid scope (not in commit_scopes.txt)
Feat(data): add feature           # Type should be lowercase
feat(data): Add feature           # Description should start lowercase
```

### Configuration

The script uses [`commit_scopes.txt`](../../commit_scopes.txt) as the source of truth for valid scopes. Update that file to add or modify allowed
scopes.

### Exit Codes

- `0`: All commits are valid
- `1`: Invalid commits found on a `pull_request` event, or when a single commit was checked
- `0`: Invalid commits found among several commits outside a `pull_request` event (warning mode for legacy commits on push)