# Dockify Architecture Planning: Worker Profiles, RBAC, and Native Build Pipeline

## Recommended Model

- Model: GPT-5.6 Sol
- Reasoning effort: High

## Objective

Inspect the current Dockify codebase and produce a detailed,
implementation-ready plan for evolving Dockify toward a developer experience
closer to Deno Deploy, while preserving Docker as the application runtime.

Do not implement any code in this task.

The plan must be based on the actual current architecture, data model, UI,
deployment flow, worker management, routing, CI/CD integration, and operational
constraints found in this repository.

## Current Context

Dockify is already used as a centralized platform for managing Docker
applications across existing VM workers.

Current infrastructure direction:

- Dockify Controller runs on a dedicated Debian VM.
- Existing VMs are registered as Dockify workers.
- The underlying infrastructure currently remains on an OVH bare-metal server
  running ESXi.
- Applications run as Docker containers.
- Dockify already manages workers, applications, routing, HTTPS, DNS automation,
  deployments, monitoring, rollback, secrets, backups, and related operations.
- VM provisioning is still performed manually outside Dockify.
- The immediate goal is not to automate ESXi or Proxmox provisioning.
- The immediate goal is to reduce the need for one VM per application by using a
  smaller number of shared worker pools.
- Dockify should remain usable for existing image-based deployments while adding
  repository-native builds.

The desired developer experience is approximately:

    Connect repository
    → select branch
    → configure environment and domain
    → build automatically
    → deploy to an appropriate worker
    → perform health validation
    → activate the new deployment
    → allow rollback

Do not attempt to clone Deno Deploy's isolate runtime. Docker remains the
workload runtime.

## Architecture Topics to Evaluate

### 1. Worker Profiles and Placement

Evaluate how Dockify should support worker classification and workload-aware
placement.

Initial conceptual profiles:

    workload=web      → worker-apps
    workload=data     → worker-data
    workload=compute  → worker-heavy

Do not assume these exact names or this exact data model are correct.

Inspect the current worker model and propose the best representation using
labels, attributes, capabilities, profiles, or placement rules.

Possible worker metadata may include:

- workload class
- environment
- region
- storage class
- persistent storage capability
- CPU class
- available capacity
- reserved capacity
- maintenance or scheduling status

The end user should not normally need to select a specific VM.

A developer-facing application configuration may instead expose profiles such
as:

- Auto
- Web Service
- Data Service
- Compute Worker

Dockify should then resolve the application requirements into a worker placement
decision.

Evaluate:

- how this fits the existing least-loaded-worker logic
- whether placement should be automatic, explicit, or both
- how manually pinned workers should work
- how capacity reservation should differ from current resource utilization
- how existing applications and workers can migrate without breaking
  compatibility
- whether resource limits should be mandatory or optional
- how worker draining and maintenance should behave
- whether data workloads should be automatically movable or treated as pinned
  because of persistent storage

### 2. UI, Roles, and Tenancy

Determine whether Dockify should remain one application UI with role-based
access or use separate administrator and developer portals.

Preferred direction unless the codebase strongly suggests otherwise:

- one backend
- one frontend codebase
- one authentication model
- role-aware navigation and authorization
- infrastructure menus hidden from non-platform administrators

Evaluate a minimal RBAC model such as:

- Platform Admin
- Project Admin
- Developer
- Viewer

Possible responsibilities:

#### Platform Admin

- register and manage workers
- configure worker profiles and capabilities
- manage build infrastructure
- manage registries
- define global policies
- inspect all projects and deployments

#### Project Admin

- manage project members
- configure domains
- manage secrets and environment variables
- configure deployment settings
- deploy and rollback

#### Developer

- trigger deployments
- inspect build and deployment logs
- restart applications
- perform permitted rollbacks

#### Viewer

- read-only access

Inspect the current authentication, authorization, UI routing, API
authorization, and ownership models.

Recommend whether tenancy should initially be:

- single organization with project-level access
- workspace/team based
- prepared for future SaaS multi-tenancy

Do not introduce public SaaS complexity unless the existing architecture makes
it low-cost and appropriate.

### 3. Deployment Source Model

Dockify currently expects an existing Docker image name when creating an
application.

Design a backward-compatible deployment source abstraction supporting at least:

#### Existing Image

The user supplies an existing OCI/Docker image:

    ghcr.io/company/application:tag

The image may continue to be built by GitHub Actions, GitLab CI, or another
external system.

#### Git Repository

Dockify handles:

- repository connection
- branch selection
- webhook processing
- checkout of an exact commit
- Docker image build
- image publication
- deployment
- logs
- rollback

Evaluate the best domain model for deployment sources, build configuration,
repository credentials, branch mapping, webhook configuration, and deployment
history.

The design must preserve current image-based applications.

### 4. Native Build Architecture

Evaluate and plan a native Dockify build pipeline.

Preferred baseline architecture:

    Git repository
        ↓
    Dockify Controller
        ↓ build job
    Dedicated Build Worker
        ↓
    Private OCI Registry
        ↓
    Runtime Worker
        ↓
    Docker container

The runtime workers should preferably only:

    pull
    → create/start candidate deployment
    → health check
    → activate routing
    → retire the previous deployment

They should not normally clone source repositories or run
`docker compose build`.

Evaluate the following alternatives against the current codebase:

1. Dedicated build VM running BuildKit and a private OCI registry.
2. BuildKit and registry on separate VMs.
3. Building directly on runtime workers using `docker compose build`.
4. Continuing to depend only on external CI.
5. A hybrid model supporting both Dockify builds and externally built images.

The likely initial recommendation is:

- hybrid deployment sources
- dedicated build worker
- BuildKit
- private OCI registry
- BuildKit and registry may initially share one VM
- separate storage areas and lifecycle policies for build cache versus registry
  artifacts

However, validate this against the actual Dockify implementation rather than
blindly accepting it.

### 5. Build Job Lifecycle

Plan the complete lifecycle of a repository-native deployment.

Expected conceptual flow:

1. Git provider webhook is received.
2. Dockify verifies the webhook signature.
3. A deployment and build job are created.
4. The build worker receives or polls the job.
5. The repository is cloned using temporary scoped credentials.
6. The exact commit SHA is checked out.
7. The configured build context and Dockerfile are used.
8. BuildKit builds the image.
9. The image is tagged with an immutable identifier.
10. The image is pushed to the configured registry.
11. Dockify records the image digest.
12. A target runtime worker is selected.
13. The worker pulls the image by digest or immutable reference.
14. A candidate container is started.
15. Health checks run.
16. Routing is switched only after successful health validation.
17. The previous deployment is retained according to rollback policy.
18. Failed candidates are removed without disrupting the current deployment.

Evaluate:

- synchronous versus queued execution
- job state machine
- retries and idempotency
- cancellation
- controller restart recovery
- worker disconnect handling
- duplicate webhook delivery
- concurrent deployment handling
- deployment locking
- log streaming
- build and deployment timeout
- failure reporting

### 6. Image Identity and Rollback

Design an immutable image and deployment identity model.

Avoid relying on `latest`.

Possible references:

    registry.internal/team/project:git-<commit-sha>
    registry.internal/team/project:deployment-<deployment-id>

The authoritative deployment record should ultimately reference an image digest:

    sha256:...

Evaluate the relationships between:

- repository
- commit SHA
- build
- image tag
- image digest
- deployment
- environment
- worker
- active release
- rollback target

Rollback should redeploy an existing retained image digest, not rebuild an older
commit.

Determine how this integrates with Dockify's existing deployment history and
rollback implementation.

### 7. Docker Compose Compatibility

Inspect how Dockify currently handles Docker Compose applications.

Plan how repository-native builds should work for:

- a single application image
- multiple locally built services
- services using upstream public images
- application services plus databases or Redis
- Compose files containing both `build:` and `image:`
- existing Compose-based Dockify applications

Preferred production principle:

- builds happen on build workers
- runtime workers receive a resolved Compose definition using immutable `image:`
  references
- runtime workers do not build source

Determine whether Dockify should:

- transform Compose build definitions
- generate an override Compose file
- create a resolved deployment manifest
- support only one buildable service initially
- support multiple build targets later
- reject unsupported build configurations with clear errors

Keep the initial scope practical.

### 8. Runtime Deployment Strategy

Evaluate the current deployment implementation and propose the appropriate
progression toward safer releases.

Possible stages:

#### Initial

- pull new image
- start container
- validate basic health
- replace old deployment

#### Improved

- blue-green deployment
- candidate container
- health-gated route switch
- connection draining
- rollback on failure

Determine what can be implemented using Dockify's current Caddy integration.

Do not assume Kubernetes-style orchestration is required.

### 9. Build and Registry Storage Management

A major concern is build cache, dangling images, unused layers, and registry
growth.

Design separate lifecycle policies for:

#### BuildKit Cache

Build cache should not be deleted after every build because it improves
subsequent build performance.

Plan automatic garbage collection based on:

- maximum disk percentage
- reserved free space
- cache age
- active versus unused cache
- configurable global defaults
- optional per-builder overrides

#### Runtime Worker Images

Runtime workers should retain:

- current active deployment
- a configurable number of previous successful deployments
- pinned deployments

Runtime workers should remove:

- dangling images
- images from deleted applications
- failed deployment images
- unused images older than a retention threshold

Do not recommend blindly scheduling:

    docker system prune -a

Dockify should identify which images remain referenced by active or retained
deployments before deletion.

#### Registry Artifacts

Design retention policies such as:

- production: current plus several previous successful releases
- staging: fewer retained releases
- previews: expire shortly after closure
- failed builds: short retention
- pinned releases: never automatically delete

Account for registry garbage collection after tags and manifests become
unreferenced.

Determine whether the initial implementation should use:

- Docker Distribution Registry
- Harbor
- an external OCI registry
- a registry abstraction supporting multiple implementations

Prefer the simplest solution compatible with the current Dockify codebase.

### 10. Build Security

Treat Dockerfile builds as potentially unsafe, even if the initial deployment is
internal.

Evaluate minimum safeguards:

- dedicated build worker
- no production Docker socket access
- no controller database access
- temporary and scoped Git credentials
- scoped registry credentials
- CPU limits
- memory limits
- disk limits
- build timeout
- concurrency limits
- isolated build workspace
- workspace cleanup
- restricted network where practical
- BuildKit secret mounts
- no build secrets passed through image arguments or persisted layers
- audit logs
- protection from malicious or accidental Dockerfiles

Clearly distinguish:

- acceptable controls for current trusted internal use
- controls needed before offering Dockify as a public multi-tenant SaaS

### 11. Build Worker Registration and Scheduling

Determine whether build workers should reuse the current worker model or be a
distinct resource type.

Possible options:

- normal workers with `capability=build`
- dedicated build-worker entity
- worker roles such as `runtime`, `build`, or both
- agent-based build execution
- SSH-based build execution using the current worker communication mechanism

Evaluate this based on Dockify's current SSH orchestration architecture.

Consider:

- build worker capacity
- max concurrent builds
- disk usage
- queue length
- builder health
- draining
- maintenance
- multiple future build workers
- affinity to registry location
- failure and retry behavior

### 12. Runtime Presets for Future Deno Deploy-like UX

Do not include runtime auto-detection in the first implementation unless it is
already straightforward.

However, prepare the architecture for future presets such as:

- Deno
- Bun
- Node.js
- Go
- Static Site

A future flow could allow a repository to deploy without supplying a Dockerfile
by generating an internal build plan.

For the initial version, likely supported modes should remain:

- existing image
- Dockerfile-based repository build

Determine what extension points should be introduced now without
over-engineering the first release.

## Required Repository Inspection

Before proposing the plan, inspect at minimum:

- README and architecture documentation
- controller entry points
- API routes
- database schema and migrations
- worker model
- application model
- deployment model
- deployment history and rollback
- GitHub/GitLab webhook implementation
- Caddy and DNS integration
- worker selection logic
- resource monitoring
- secrets/config management
- frontend routes and components
- authentication and authorization
- tests
- installation and upgrade flow

Do not base the plan only on README claims.

Identify where the repository implementation differs from the documented design.

## Scope Control

Do not implement code.

Do not rewrite Dockify from scratch.

Do not introduce Kubernetes, Nomad, Proxmox API integration, or ESXi automation
unless the repository inspection proves one is directly required for the
proposed changes.

Do not design a public SaaS billing platform.

Do not require two separate frontend codebases.

Do not remove the existing image-based deployment flow.

Prefer incremental changes that preserve current users and deployments.

## Required Output

Create a planning document at:

    docs/plans/native-build-worker-placement-rbac-plan.md

The document must include:

1. Executive summary
2. Current-state findings from the repository
3. Gaps between current behavior and desired behavior
4. Key architectural decisions
5. Recommended target architecture
6. Alternatives considered and rejected
7. Data model changes
8. API changes
9. UI and RBAC changes
10. Worker profile and placement design
11. Deployment source abstraction
12. Build worker and registry design
13. Build/deployment state machine
14. Docker Compose compatibility strategy
15. Image identity and rollback strategy
16. Garbage collection and retention policies
17. Security model
18. Backward compatibility and migration
19. Failure recovery and idempotency
20. Observability requirements
21. Testing strategy
22. Documentation changes
23. Phased implementation plan
24. File-by-file implementation map
25. Risks, unresolved decisions, and recommended defaults
26. Acceptance criteria for each phase

## Planning Quality Requirements

The plan must be specific enough that a different implementation agent can
execute it without repeating the architecture investigation.

For each proposed change, identify:

- existing files or packages involved
- new files or packages likely required
- database changes
- API changes
- frontend changes
- background jobs or state machines
- migration impact
- tests required
- operational impact

Clearly label each statement as one of:

- repository-confirmed fact
- recommended design
- unresolved decision

Where possible, reference exact files, functions, types, routes, database
tables, and UI components from the repository.

Avoid generic PaaS advice that is not grounded in Dockify's current codebase.

## Final Response

After writing the plan, return a concise summary containing:

- the planning document path
- the most important current-state findings
- the recommended architecture
- the proposed implementation phases
- the highest-risk areas
- any decisions that still require owner input

Do not modify application code, database migrations, deployment logic, or
infrastructure configuration in this task.
