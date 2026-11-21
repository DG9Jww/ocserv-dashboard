# Ocserv Dashboard repository notes

- Start with `TODO.md` when the user points to the repository backlog.
- For Go work, read `backend/.agents/skills/golang/SKILL.md` and the applicable API, persistence, runtime, or testing skill and its references. Keep the controller → usecase → repository boundaries. Run Go checks from `backend/`, where `go.mod` lives.
- For frontend work, read `web/admin/.agents/PROJECT_STRUCTURE.md` or `web/customer/.agents/PROJECT_STRUCTURE.md` and the relevant skills under that app. Both use Vue 3, TypeScript, API services, generated schemas, and seven locales. Keep the admin and customer applications independent.
- Shell deployment, Docker, server maintenance, and release notes have their own skills under `.agents/skills/`. `setup.sh` uses `deploy/docker/Dockerfile` for both node roles or delegates to the corresponding systemd installer.
- The structured Ocserv configuration API saves `/etc/ocserv/ocserv.conf`. Service actions live in the separate service-control API. Docker firewall initialization lives in `deploy/docker/entrypoint.sh`.
- Each Agent has its own Ocserv runtime, database, and token. The log worker records session events, accounts for traffic, and enforces quotas. The expiry worker enforces user expiration. Preserve both workers when reducing Agent resource use.
