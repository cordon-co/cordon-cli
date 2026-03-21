# Cordon — Current Task

## Summary

Completed: **Zone → File Rule Rename** — renamed all references to "zone" to "file rule" across the entire codebase. CLI subcommand `cordon zone` became `cordon file`. All Go identifiers (Zone → FileRule, ZoneType → FileType, ZoneAuthority → FileAuthority, ZoneForPath → FileRuleForPath, AddZone → AddFileRule, ListZones → ListFileRules, RemoveZone → RemoveFileRule, StandardGuardrailZones → StandardGuardrailFileRules). DB table `zones` became `file_rules` with columns `file_access`/`file_authority`. Passes table `zone_id` became `file_rule_id`. Audit log `zone_id` became `file_rule_id`, event types `zone_add`/`zone_remove` became `file_add`/`file_remove`. Directory `cli/cmd/zone/` became `cli/cmd/file/`. All docs updated. Requirements renamed ZON-* to FIL-*.

Previously completed: **Command Rule Type/Authority Refactor (CMD-07)** — allow/deny + standard/guardian on command rules, `--allow` flag, removed reason column, removed backward-compat migrations.

Previously completed: **File Rule Type / Authority Refactor (FIL-08)** — allow/deny access + standard/guardian authority on file rules.

Previously completed: **File Rule Read Prevention + Guardrail File Rules** — `prevent_read`/`prevent_write` columns, hook enforcement for read tools, `--prevent-read` flag, standard guardrail file rules (FIL-07, SAF-02).

Previously completed: **Command Rules** — policy-based enforcement of shell commands (CMD-01 through CMD-06, SAF-01 in progress).

Previously completed: **Tests** — store-level unit tests (matching/enforcement logic) and CLI integration tests (CRUD lifecycles). 17 tests across `cli/internal/store/` and `cli/tests/`. Runner at `cli/scripts/test.sh`, CI at `.github/workflows/test.yml`.

## Next Steps

1. **SAF-01 (remaining)** — add destructive command built-in rules (`git reset --hard*`, `git push --force*`, `rm -rf /*`)
2. **HOK-06** — `cordon init` writes `.codex/config.toml`
3. **CLI-03/04** — `cordon login` / `cordon logout`
4. **CLI-05** — `cordon status`
5. **INT-01..06** — `cordon check` integrity verification
