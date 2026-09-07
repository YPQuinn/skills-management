# baseline-cold

- home: `/private/tmp/skillctl-dev-home`
- remote: `https://github.com/mattpocock/skills`
- binary: `dist/skillctl` at `c261cec` (pre-change)
- git-cache deleted before init: yes

| step | seconds |
| --- | ---: |
| source add | **failed** |
| skill import --all | not reached |
| source check | not reached |

## Failure

`source add` cloned the partial (`blob:none`) cache, then died while reading Skill trees:

```text
reading Skill tree: git cat-file --batch: fatal: unable to access
'https://github.com/mattpocock/skills.git/': LibreSSL SSL_connect:
SSL_ERROR_SYSCALL in connection to github.com:443
fatal: could not fetch <oid> from promisor remote
```

A separate reproduction listed 37 `SKILL.md` blobs. Sequential `cat-file` of the first few succeeded; a single `cat-file --batch` of all 37 died around object 24 with the same SSL error. Lazy promisor fetches open many short HTTPS connections; on this host that is both slow and unreliable.

`git fetch --no-filter origin HEAD` after the same partial clone made every `SKILL.md` blob local in one pack (~1.2s). A `--bare --single-branch --no-tags` clone of the same repo completed in ~3s at 2.3MiB.

## Implication

Tickets 02–05 are not optional polish. The current Git path cannot finish add/import against this repository from a cold cache on this machine.
