# Build Docker Action

A reusable GitHub Action for building and pushing multi-architecture Docker images with advanced features like caching, SemVer tagging, and support for multiple registries.

## Features

- ✅ **Multi-platform builds** using Docker Buildx (linux/amd64, linux/arm64)
- 🏷️ **Automatic SemVer tagging** based on Git tags
- 🚀 **Build caching** for faster rebuilds using GitHub Actions cache
- 🐳 **Multiple registry support** (Docker Hub, GHCR, custom registries)
- 🔄 **Conditional pushing** (build-only mode for testing)
- 📋 **Rich metadata** with OpenContainer labels
- 🎯 **Matrix builds** for multiple Dockerfiles

## Usage

### Basic Usage

```yaml
- name: Build and push Docker image
  uses: ./.github/actions/build-docker
  with:
    dockerfile: docker/Dockerfile
    image-name: myorg/myapp
    registry: docker.io
    username: ${{ secrets.DOCKER_USERNAME }}
    password: ${{ secrets.DOCKER_PASSWORD }}
    push: 'true'
```

### Advanced Usage with Matrix Strategy

```yaml
jobs:
  docker:
    strategy:
      matrix:
        image: 
          - {file: 'docker/Dockerfile', name: 'myorg/myapp'}
          - {file: 'docker/demo/Dockerfile', name: 'myorg/myapp-demo'}
    steps:
      - uses: actions/checkout@v4
      
      - name: Build and push
        uses: ./.github/actions/build-docker
        with:
          dockerfile: ${{ matrix.image.file }}
          image-name: ${{ matrix.image.name }}
          registry: docker.io
          username: ${{ secrets.DOCKER_USERNAME }}
          password: ${{ secrets.DOCKER_PASSWORD }}
          push: ${{ github.ref_type == 'tag' }}
          build-args: |
            VERSION=${{ github.ref_name }}
            BUILD_DATE=${{ steps.date.outputs.date }}
```

## Inputs

| Input | Description | Required | Default |
|-------|-------------|----------|---------|
| `dockerfile` | Path to Dockerfile (relative to repo root) | ✅ | - |
| `image-name` | Docker image name (without registry prefix) | ✅ | - |
| `registry` | Docker registry (docker.io, ghcr.io, etc.) | ❌ | `docker.io` |
| `username` | Docker registry username | ✅ | - |
| `password` | Docker registry password/token | ✅ | - |
| `platforms` | Target platforms for multi-arch build | ❌ | `linux/amd64,linux/arm64` |
| `push` | Whether to push the image (true/false) | ❌ | `false` |
| `build-args` | Build arguments (KEY=VALUE, one per line) | ❌ | - |
| `cache-scope` | Cache scope for build cache | ❌ | `default` |

## Outputs

| Output | Description |
|--------|-------------|
| `image-id` | Image ID of the built image |
| `digest` | Image digest of the built image |
| `metadata` | Build result metadata |

## Tag Strategy

The action automatically applies tags based on the trigger:

### For Git Tags (Releases)

- `1.2.3` - Exact version
- `1.2` - Minor version
- `1` - Major version
- `latest` - Latest stable release (excludes pre-releases)

**Note**: Pre-release versions (e.g., `1.2.3-beta.1`, `2.0.0-rc.1`) will only get the exact version tag, not `latest` or major/minor tags.

### For Branch Pushes

- `main` or `master` → branch name (no `latest` tag)
- Other branches → branch name as tag

### For Pull Requests

- `pr-123` - PR number

### Always Applied

- `sha-abc1234` - Short commit SHA

## Registry Examples

### Docker Hub

```yaml
with:
  registry: docker.io
  username: ${{ secrets.DOCKER_USERNAME }}
  password: ${{ secrets.DOCKER_PASSWORD }}
```

### GitHub Container Registry (GHCR)

```yaml
with:
  registry: ghcr.io
  image-name: myorg/myapp  # Same name as Docker Hub
  username: ${{ github.actor }}
  password: ${{ secrets.GITHUB_TOKEN }}
```

**Note**: Use consistent image names across registries for better user experience.

### Custom Registry

```yaml
with:
  registry: my-registry.com
  username: ${{ secrets.CUSTOM_REGISTRY_USER }}
  password: ${{ secrets.CUSTOM_REGISTRY_TOKEN }}
```

## Build-Only Mode

To test builds without pushing (useful in PR workflows):

```yaml
- name: Test Docker build
  uses: ./.github/actions/build-docker
  with:
    dockerfile: docker/Dockerfile
    image-name: myorg/myapp
    registry: docker.io
    username: dummy  # Not used when push=false
    password: dummy  # Not used when push=false
    push: 'false'
```

## Build Arguments

Pass build arguments to Docker:

```yaml
with:
  build-args: |
    VERSION=${{ github.ref_name }}
    GIT_HASH=${{ github.sha }}
    BUILD_DATE=${{ steps.date.outputs.date }}
    ENVIRONMENT=production
```

## Caching

The action uses GitHub Actions cache for Docker layer caching. Use different `cache-scope` values for different build contexts:

```yaml
with:
  cache-scope: main-app     # For main application
  # vs
  cache-scope: demo-app     # For demo application
```

## Security Notes

- Registry credentials are only used when `push: 'true'`
- Build-only mode doesn't require valid credentials
- Use repository secrets for sensitive data
- GHCR automatically uses `GITHUB_TOKEN` which has appropriate permissions

## Conditional Pushing

Common patterns for when to push images:

```yaml
# Push only on tags (releases)
push: ${{ github.ref_type == 'tag' }}

# Push on tags or main branch
push: ${{ github.ref_type == 'tag' || github.ref_name == 'main' }}

# Push only on releases (not pre-releases)
push: ${{ github.ref_type == 'tag' && !contains(github.ref_name, 'beta') }}
```

## Workflow Integration

### Avoiding Conflicts

When using this action in multiple workflows, be careful about triggers to avoid conflicts:

```yaml
# ✅ Good: Release workflow (official releases)
on:
  release:
    types: [created]
# Uses: push: 'true' for Docker Hub

# ✅ Good: Build workflow (testing)
on:
  pull_request:
# Uses: push: 'false' (build-only)

# ❌ Bad: Both workflows triggering on same tag
# Don't use both of these:
on: { push: { tags: ['v*'] } }  # Workflow 1
on: { release: { types: [created] } }  # Workflow 2 (GitHub creates tag automatically)
```

### Recommended Setup

- **Release Workflow**: Handle official releases to Docker Hub
- **Build Workflow**: Test builds on PRs (no push)  
- **Multi-Registry Workflow**: Manual builds for alternative registries
