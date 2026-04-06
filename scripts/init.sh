#!/usr/bin/env bash
# Bash script to initialize Kopiochi boilerplate for a new project
# Usage: ./scripts/init.sh --project-name myapi --author "John Doe"

set -e

# Colors
CYAN='\033[0;36m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
GRAY='\033[0;37m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Defaults
PROJECT_NAME=""
AUTHOR=$(whoami 2>/dev/null || echo "developer")
MODULE_PATH=""
DB_NAME=""
REMOVE_EXAMPLE=true
RESET_GIT=true

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --project-name)
            PROJECT_NAME="$2"
            shift 2
            ;;
        --author)
            AUTHOR="$2"
            shift 2
            ;;
        --module-path)
            MODULE_PATH="$2"
            shift 2
            ;;
        --db-name)
            DB_NAME="$2"
            shift 2
            ;;
        --keep-example)
            REMOVE_EXAMPLE=false
            shift
            ;;
        --keep-git)
            RESET_GIT=false
            shift
            ;;
        --help)
            echo "Usage: ./scripts/init.sh --project-name NAME [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --project-name NAME      Project name (required)"
            echo "  --author NAME            Author name (default: current user)"
            echo "  --module-path PATH       Go module path (default: github.com/AUTHOR/NAME)"
            echo "  --db-name NAME           Database name (default: lowercase project name)"
            echo "  --keep-example           Keep example User CRUD domain"
            echo "  --keep-git              Keep existing Git history"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Validate
if [ -z "$PROJECT_NAME" ]; then
    echo -e "${RED}Error: --project-name is required${NC}"
    exit 1
fi

# Set defaults
MODULE_PATH="${MODULE_PATH:-github.com/$AUTHOR/$PROJECT_NAME}"
DB_NAME="${DB_NAME:-$(echo $PROJECT_NAME | tr '[:upper:]' '[:lower:]')}"

# Get root directory
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo -e "${CYAN}========================================"
echo "  Kopiochi Project Initializer"
echo "========================================${NC}"
echo ""
echo -e "${YELLOW}Project Name : $PROJECT_NAME${NC}"
echo -e "${YELLOW}Author       : $AUTHOR${NC}"
echo -e "${YELLOW}Module Path  : $MODULE_PATH${NC}"
echo -e "${YELLOW}DB Name      : $DBName${NC}"
echo -e "${YELLOW}Remove Example: $REMOVE_EXAMPLE${NC}"
echo -e "${YELLOW}Reset Git    : $RESET_GIT${NC}"
echo ""

read -p "Continue? (y/n): " confirm
if [ "$confirm" != "y" ]; then
    echo -e "${RED}Aborted.${NC}"
    exit 1
fi

OLD_MODULE="github.com/sujanto-gaws/kopiochi"
OLD_NAME="kopiochi"

# Function to replace text in file
replace_in_file() {
    local file="$1"
    local old="$2"
    local new="$3"
    
    if [ -f "$file" ]; then
        if [[ "$OSTYPE" == "darwin"* ]]; then
            sed -i '' "s|$(echo $old | sed 's/[\/&]/\\&/g')|$(echo $new | sed 's/[\/&]/\\&/g')|g" "$file"
        else
            sed -i "s|$(echo $old | sed 's/[\/&]/\\&/g')|$(echo $new | sed 's/[\/&]/\\&/g')|g" "$file"
        fi
        echo -e "${GRAY}  Updated: $file${NC}"
    fi
}

echo ""
echo -e "${GREEN}[1/5] Updating module paths...${NC}"

# Update go.mod
replace_in_file "$ROOT_DIR/go.mod" "$OLD_MODULE" "$MODULE_PATH"

# Update all Go source files
find "$ROOT_DIR" -name "*.go" -type f | while read -r file; do
    replace_in_file "$file" "$OLD_MODULE" "$MODULE_PATH"
done

echo ""
echo -e "${GREEN}[2/5] Updating configuration...${NC}"

# Update config files
replace_in_file "$ROOT_DIR/config/default.yaml" "$OLD_NAME" "$PROJECT_NAME"
replace_in_file "$ROOT_DIR/config/default.yaml" "kopiochi" "$DB_NAME"
replace_in_file "$ROOT_DIR/.env.example" "kopiochi" "$DB_NAME"

# Update README
replace_in_file "$ROOT_DIR/README.md" "Kopiochi" "$PROJECT_NAME"
replace_in_file "$ROOT_DIR/README.md" "kopiochi" "$PROJECT_NAME"
replace_in_file "$ROOT_DIR/README.md" "sujanto-gaws/kopiochi" "$AUTHOR/$PROJECT_NAME"

# Update PLUGIN_GUIDE.md
replace_in_file "$ROOT_DIR/PLUGIN_GUIDE.md" "Kopiochi" "$PROJECT_NAME"

# Update cmd/api/main.go (CLI name)
replace_in_file "$ROOT_DIR/cmd/api/main.go" "kopiochi" "$PROJECT_NAME"

# Update generator defaults
replace_in_file "$ROOT_DIR/cmd/generator/main.go" "$OLD_MODULE" "$MODULE_PATH"
replace_in_file "$ROOT_DIR/cmd/generator/README.md" "$OLD_MODULE" "$MODULE_PATH"

echo ""
echo -e "${GREEN}[3/5] Updating copyright information...${NC}"

CURRENT_YEAR=$(date +%Y)
replace_in_file "$ROOT_DIR/LICENSE" "Sujanto" "$AUTHOR"
replace_in_file "$ROOT_DIR/LICENSE" "2026" "$CURRENT_YEAR"

echo ""
echo -e "${GREEN}[4/5] Tidying Go module...${NC}"

cd "$ROOT_DIR"
go mod tidy

if [ "$REMOVE_EXAMPLE" = true ]; then
    echo ""
    echo -e "${GREEN}[5/5] Removing example domain (user CRUD)...${NC}"
    
    # Remove example domain files
    rm -rf "$ROOT_DIR/internal/domain/user"
    echo -e "${GRAY}  Removed: internal/domain/user${NC}"
    
    rm -rf "$ROOT_DIR/internal/application/user"
    echo -e "${GRAY}  Removed: internal/application/user${NC}"
    
    rm -f "$ROOT_DIR/internal/infrastructure/http/handlers/user.go"
    echo -e "${GRAY}  Removed: internal/infrastructure/http/handlers/user.go${NC}"
    
    rm -f "$ROOT_DIR/internal/infrastructure/persistence/repository/user.go"
    echo -e "${GRAY}  Removed: internal/infrastructure/persistence/repository/user.go${NC}"
fi

if [ "$RESET_GIT" = true ]; then
    echo ""
    echo -e "${GREEN}Resetting Git history...${NC}"
    
    cd "$ROOT_DIR"
    
    # Remove .git directory
    if [ -d ".git" ]; then
        rm -rf .git
        echo -e "${GRAY}  Removed .git directory${NC}"
    fi
    
    # Reinitialize
    git init > /dev/null 2>&1
    echo -e "${GRAY}  Initialized new Git repository${NC}"
fi

echo ""
echo -e "${CYAN}========================================"
echo "  Initialization Complete! 🎉"
echo "========================================${NC}"
echo ""
echo -e "${YELLOW}Next steps:${NC}"
echo -e "  1. Review the changes:"
echo -e "${GRAY}     git status${NC}"
echo ""
echo -e "  2. Make initial commit:"
echo -e "${GRAY}     git add .${NC}"
echo -e "${GRAY}     git commit -m 'Initial commit: $PROJECT_NAME boilerplate'${NC}"
echo ""
echo -e "  3. Start developing:"
echo -e "${GRAY}     go run ./cmd/api serve${NC}"
echo ""
echo -e "  4. Generate new domains:"
echo -e "${GRAY}     go run ./cmd/generator -domain Product -fields 'name:string,price:float64'${NC}"
echo ""
