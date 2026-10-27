# dconsole-acquia

A [dconsole](https://github.com/dconsole/dconsole) plugin that reaches
Acquia Cloud environments through
[acli](https://github.com/acquia/cli).

```yaml
# .dconsole.yml
project: my-acquia-site
default_env: dev
_defaults:
  handler:
    type: acquia
    acquia:
      alias: myapp.${env}        # just the environment id
dev:  { acquia: { alias: myapp.dev  } }
test: { acquia: { alias: myapp.test } }
prod: { acquia: { alias: myapp.prod } }
```

```sh
dconsole @my-acquia-site.dev cr         # drush cr on dev
dconsole @my-acquia-site.prod sql:dump  # streams the dump through stdout
dconsole @my-acquia-site.dev sh         # interactive shell at the docroot
dconsole sql:sync @my-acquia-site.prod @my-acquia-site.local   # pull-down
```

Underneath, every call becomes `acli remote:ssh <alias> -- <quoted-command>`
on the matching Cloud server. acli handles the SSH URL rotation,
auth tokens, and the `cd /var/www/html/<alias>` step — this plugin is
the glue that teaches dconsole to drive it.

## Install

Requires [acli](https://github.com/acquia/cli) on `$PATH` and
dconsole v0.5.14+.

```sh
# From a release:
dconsole plugin install acquia

# From source (while developing):
go build -o dconsole-acquia .
mkdir -p ~/.dconsole/plugins
ln -sf "$PWD/dconsole-acquia" ~/.dconsole/plugins/dconsole-acquia
```

Verify:

```sh
dconsole plugin list      # should include `acquia`
dconsole transport:list   # should include `acquia` as `ok`
```

## Configuration

| Key          | Required | Default               | Notes                                                                 |
|--------------|----------|-----------------------|-----------------------------------------------------------------------|
| `alias`      | yes      | —                     | Acquia environment id, e.g. `myapp.prod` or an environment UUID.      |
| `cmd_prefix` | no       | `["acli","remote:ssh"]` | Override the acli subcommand (e.g. `["acli","remote:drush"]`) or point at a wrapper script. |

The alias format matches acli's own `<app>.<env>` convention, which
happens to mirror dconsole's `@site.env` — so for an acli alias like
`myapp.prod`, naming your dconsole site `myapp` keeps the two in sync.

## What the plugin does and doesn't

- **Does**: proxy any remote command (drush, drupal, cat, …) through
  acli. `sql:sync` and `rsync` work because the plugin exposes a generic
  shell, so dconsole's existing dump+load pipeline flows through.
- **Doesn't** (yet): wrap acli's higher-level verbs (`pull:database`,
  `pull:files`, `log:tail`). Those would plug in as provider verbs
  (`DumpFor`, `FilesDownload`) in a future release.

## Local development

The repo includes a `replace` directive in `go.mod` pointing at a
sibling `../dconsole` checkout, so changes to the plugin wire protocol
flow through without a release cycle. Remove or comment the `replace`
line before tagging a release.

```sh
go test ./...
go build -o dconsole-acquia .
```

## License

Apache License 2.0. See [LICENSE](LICENSE).
