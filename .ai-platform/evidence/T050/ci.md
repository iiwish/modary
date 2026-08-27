# v0.3.0-alpha.2 Hosted Verification

- Candidate main CI: https://github.com/iiwish/modary/actions/runs/33041481326
- Release tag CI: https://github.com/iiwish/modary/actions/runs/33042244822
- GitHub prerelease: https://github.com/iiwish/modary/releases/tag/v0.3.0-alpha.2

The candidate main run passed quality, copied-profile, operational-provider,
and Darwin ARM64 jobs. The tag run repeated those gates and passed tag-mode
preflight, replacement-free five-module consumption, released-source container
acceptance, and final source-stability verification in its release job.

The release-record commit contains documentation and evidence only. Delivery
closure additionally requires its hosted `main` run to pass before the goal is
marked complete.
