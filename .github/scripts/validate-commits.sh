#!/bin/bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Path to the commit scopes file
SCOPES_FILE="commit_scopes.txt"

# Valid commit types according to Conventional Commits
VALID_TYPES="feat fix docs style refactor test chore ci perf build"

# Function to check if a type is valid
is_valid_type() {
    local type="$1"
    for valid in $VALID_TYPES; do
        if [[ "$type" == "$valid" ]]; then
            return 0
        fi
    done
    return 1
}

# Function to extract valid scopes from commit_scopes.txt
get_valid_scopes() {
    # Read the file and extract scopes (lines that are not comments or empty)
    # Scopes can contain letters, numbers, hyphens, and forward slashes
    grep -E '^[a-zA-Z0-9/_-]+' "$SCOPES_FILE" | awk '{print $1}' | grep -v '^#' | sort -u
}

# Function to validate a single commit message
validate_commit() {
    local commit_hash="$1"
    local commit_msg="$2"
    local errors=()
    
    # Skip merge commits, revert commits, and automated commits
    if [[ "$commit_msg" =~ ^(Merge|Revert|Auto.merge) ]]; then
        echo -e "${GREEN}✓${NC} Skipping special commit: $commit_hash"
        return 0
    fi
    
    # Skip commits that are automated (dependabot, etc.)
    if [[ "$commit_msg" =~ ^(build\(deps\)|chore\(deps\)) ]]; then
        echo -e "${GREEN}✓${NC} Skipping automated commit: $commit_hash"  
        return 0
    fi
    
    # Check for Conventional Commits format with optional scope
    # Format: type[(scope)]: description
    if [[ "$commit_msg" =~ ^([a-z]+)(\(([^\)]+)\))?:\ (.+) ]]; then
        local type="${BASH_REMATCH[1]}"
        local scope="${BASH_REMATCH[3]}"  # Optional
        local description="${BASH_REMATCH[4]}"
        
        # Validate type
        if ! is_valid_type "$type"; then
            errors+=("Invalid type '$type'. Valid types are: $VALID_TYPES")
        fi
        
        # Validate scope if present
        if [[ -n "$scope" ]]; then
            # Check if scope exists in the allowed list
            if ! get_valid_scopes | grep -Fxq "$scope"; then
                errors+=("Invalid scope '$scope'. Check commit_scopes.txt for valid scopes")
            fi
        fi
        
        # Validate description
        if [[ -z "$description" ]]; then
            errors+=("Missing description after colon")
        elif [[ "${description:0:1}" =~ [A-Z] ]]; then
            errors+=("Description should start with lowercase letter")
        fi
        
    else
        errors+=("Does not follow Conventional Commits format: type[(scope)]: description")
    fi
    
    # Report results
    if [[ ${#errors[@]} -eq 0 ]]; then
        echo -e "${GREEN}✓${NC} Valid: $commit_hash - $commit_msg"
        return 0
    else
        echo -e "${RED}✗${NC} Invalid: $commit_hash - $commit_msg"
        for error in "${errors[@]}"; do
            echo -e "  ${YELLOW}→${NC} $error"
        done
        return 1
    fi
}

# Main validation logic
main() {
    echo "========================================="
    echo "Validating commit messages..."
    echo "========================================="
    
    # Check if commit_scopes.txt exists
    if [[ ! -f "$SCOPES_FILE" ]]; then
        echo -e "${RED}Error:${NC} $SCOPES_FILE not found!"
        exit 1
    fi
    
    local failed=0
    local total=0
    
    # Determine which commits to check based on context
    if [[ -n "$GITHUB_EVENT_NAME" ]]; then
        # Running in GitHub Actions
        if [[ "$GITHUB_EVENT_NAME" == "pull_request" ]]; then
            # For PRs, check all commits between base and head
            echo "Checking commits in pull request..."
            
            # Fetch the base branch to ensure we have the commit history
            git fetch origin "$GITHUB_BASE_REF" --depth=50 2>/dev/null || true
            
            # Get commits between base and head
            local base_sha
            base_sha=$(git merge-base "origin/$GITHUB_BASE_REF" HEAD 2>/dev/null || echo "origin/$GITHUB_BASE_REF")
            commits=$(git log --pretty=format:'%H %s' "$base_sha"..HEAD 2>/dev/null || git log --pretty=format:'%H %s' HEAD~10..HEAD)
        else
            # For push events, check recent commits (up to 10)
            echo "Checking recent commits on push..."
            commits=$(git log --pretty=format:'%H %s' -10)
        fi
    else
        # Running locally - check commits not in origin/develop
        echo "Checking local commits not in origin/develop..."
        git fetch origin develop --depth=50 2>/dev/null || true
        commits=$(git log --pretty=format:'%H %s' origin/develop..HEAD 2>/dev/null || git log --pretty=format:'%H %s' -10)
    fi
    
    # Debug: show what commits we're checking
    echo "Found commits to check:"
    echo "$commits"
    echo "========================================="
    
    # Validate each commit
    while IFS= read -r line; do
        [[ -z "$line" ]] && continue
        
        hash=$(echo "$line" | cut -d' ' -f1)
        msg=$(echo "$line" | cut -d' ' -f2-)
        
        echo "Processing commit: $hash - $msg"
        ((total++))
        if ! validate_commit "$hash" "$msg"; then
            ((failed++))
            echo "  ^^ This commit failed validation"
        fi
    done <<< "$commits"
    
    # Summary
    echo "========================================="
    if [[ $total -eq 0 ]]; then
        echo -e "${YELLOW}No commits to validate${NC}"
        exit 0
    elif [[ $failed -eq 0 ]]; then
        echo -e "${GREEN}All $total commit(s) are valid!${NC}"
        exit 0
    else
        echo -e "${RED}$failed out of $total commit(s) are invalid${NC}"
        echo ""
        
        # For push events, be more lenient with older commits
        if [[ -n "$GITHUB_EVENT_NAME" && "$GITHUB_EVENT_NAME" == "push" && $total -gt 1 ]]; then
            echo -e "${YELLOW}Note: This is a push event with multiple commits.${NC}"
            echo -e "${YELLOW}Some commits may predate the commit convention enforcement.${NC}"
            echo -e "${YELLOW}Future commits should follow the format below.${NC}"
            echo ""
        fi
        
        echo "Please fix the commit messages to follow Conventional Commits format:"
        echo "  type(scope): description"
        echo ""
        echo "Examples:"
        echo "  feat(data/mongo): add cursor pagination support"
        echo "  fix(transport/http): handle nil pointer in retry logic"
        echo "  docs(README): update installation instructions"
        echo ""
        echo "Valid types: $VALID_TYPES"
        echo "Valid scopes: See commit_scopes.txt"
        
        # Exit with error only for PR events or single commits on push
        if [[ "$GITHUB_EVENT_NAME" == "pull_request" || $total -eq 1 ]]; then
            exit 1
        else
            echo ""
            echo -e "${YELLOW}Warning: Commit validation failed, but continuing due to push context${NC}"
            exit 0
        fi
    fi
}

# Run main function
main "$@"