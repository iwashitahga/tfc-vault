# tfc-vault

Keep Terraform API tokens in a locked macOS keychain instead of a plaintext
file, and hand them to Terraform only for the lifetime of a single command.
Built along the same lines as [aws-vault](https://github.com/99designs/aws-vault).

`terraform login` writes your token to `~/.terraform.d/credentials.tfrc.json` as
cleartext JSON. The file is mode 0600, but it is not encrypted, so the token
travels into every backup, every synced home directory and every `cat` of that
path. Worse, any process running as you can read it at any time.

tfc-vault keeps the token in a keychain of its own that relocks on sleep and
after fifteen idle minutes. While it is locked nothing can read the token, and
when Terraform needs it macOS asks for the keychain passphrase.

macOS only. There is no other keychain backend.

## Install

```sh
brew install iwashitahga/tap/tfc-vault
```

or

```sh
go install github.com/iwashitahga/tfc-vault@latest
```

Then wire it into Terraform:

```sh
tfc-vault install
```

That links the credentials helper, writes the `credentials_helper` block into
`~/.terraformrc`, and offers to move any existing plaintext tokens into the
keychain. After it finishes, plain `terraform plan` works with no wrapper.

## Usage

### Store a token

```sh
tfc-vault add                      # a token for app.terraform.io
tfc-vault add -open                # open the token page in a browser first
tfc-vault add tfe.example.com      # a self-hosted Terraform Enterprise
tfc-vault add -profile personal    # a second token for the same host
```

The first `add` creates the `tfc-vault` keychain and asks you to choose a
passphrase for it. That passphrase is what macOS will prompt for later, so it
should not be one you would type into a random dialog without reading it.

The token itself is not echoed, and it is checked against the host before being
stored. Pass `-no-verify` to skip that check.

Already logged in? Move the existing file into the keychain:

```sh
tfc-vault import           # copy the tokens, leave the file alone
tfc-vault import -purge    # copy them, then overwrite and delete the file
```

Those tokens have been on disk in the clear, so rotate them in the web UI
afterwards.

### Run one command with the token

```sh
tfc-vault exec app.terraform.io -- terraform plan
tfc-vault exec -tfe-token app.terraform.io -- terraform plan
```

This sets `TF_TOKEN_<host>` in the child environment and replaces itself with the
command via `exec(2)`. Nothing is left in your own shell. `-tfe-token` also sets
`TFE_TOKEN`, which the `tfe` provider and other go-tfe tools read. `TFE_TOKEN` is
removed from the child environment when the flag is absent, so a stale value
inherited from your shell cannot reach the command.

A profile is named after its host unless you passed `-profile`, so the hostname
usually works as the profile name.

### Let Terraform ask for it

After `tfc-vault install`, Terraform calls tfc-vault whenever it needs a token,
and `terraform login` stores new tokens in the keychain rather than the file.

Terraform reads `credentials.tfrc.json` **before** it consults a credentials
helper, and skips the helper entirely for a host that file already covers. The
helper therefore does nothing until that file is gone. `tfc-vault install`
checks for it and offers to migrate.

To undo, run `tfc-vault install -uninstall` and delete the `credentials_helper`
block from `~/.terraformrc`.

### Locking

```sh
tfc-vault lock      # relock now
```

The keychain relocks on sleep and after fifteen idle minutes. Nothing else is
needed: when a token is read from a locked keychain, macOS raises its own
passphrase dialog, and the command carries on once you answer it.

Letting macOS ask is deliberate. The dialog is drawn by a separate system
process, so the passphrase never enters tfc-vault's memory, and the system says
which application is requesting access. A passphrase typed at a prompt printed
by tfc-vault would have neither property, and any program can print the same
prompt.

`tfc-vault unlock` reads the passphrase in the terminal instead. It exists for
sessions where macOS cannot draw a dialog, such as ssh. Prefer the dialog
whenever there is one.

To change how long it stays unlocked, use the standard keychain tool. This sets
an hour, and keeps the lock on sleep:

```sh
security set-keychain-settings -l -u -t 3600 ~/Library/Keychains/tfc-vault.keychain-db
```

Set `TFC_VAULT_KEYCHAIN=login` to use the login keychain instead, if you would
rather not manage a second passphrase. That is weaker; see below. Moving
profiles between keychains:

```sh
tfc-vault migrate -from login -remove
```

### Everything else

```sh
tfc-vault list        # profile names
tfc-vault list -v     # also the host and environment variable for each
tfc-vault get NAME    # the raw token on stdout
tfc-vault env NAME    # shell export lines
tfc-vault remove NAME # delete a profile
```

## What this does and does not protect

The token is not on the filesystem in cleartext, so it no longer appears in
backups, synced home directories or dotfile repositories.

More importantly, it lives in a keychain that is locked most of the time. While
that keychain is locked, nothing can read the token. Not another process, not a
script running as you, not this:

```sh
security find-generic-password -s tfc-vault -a app.terraform.io -w
```

That is the part the login keychain cannot give you. The login keychain unlocks
when you log in and stays unlocked for the whole session, and its per-item access
control lists did not restrict other applications in testing, whether or not the
storing binary was code-signed. If you set `TFC_VAULT_KEYCHAIN=login`, any
process running as you can read the token at any time, exactly as it could read
the plaintext file.

While the dedicated keychain is unlocked the same is true, so the auto-lock
interval is the size of the window. Shorten it if you want a smaller one.

`tfc-vault exec` and the `TF_TOKEN_*` mechanism put the token into the child
process environment, where every provider plugin, `local-exec` script and other
subprocess Terraform starts can read it. That is inherent to how Terraform
accepts tokens, and it matches the tradeoff `aws-vault exec` makes.

## Related work

Several credentials helpers already store Terraform tokens in the system
keychain, and if the helper is all you need, one of them may suit you better:

- [bendrucker/terraform-credentials-keychain](https://github.com/bendrucker/terraform-credentials-keychain), Go, with signed and notarised releases.
- [alisdair/terraform-credentials-keychain](https://github.com/alisdair/terraform-credentials-keychain), a shell script.
- [terracreds](https://github.com/tonedefdev/terracreds), cross-platform, with third-party vault backends.
- [terraform-credentials-op](https://github.com/razorsedge/terraform-credentials-op), backed by 1Password.

tfc-vault adds the parts those do not cover: `aws-vault`-style `exec`, named
profiles, and a dedicated keychain created and locked for you rather than
configured by hand.

## Design notes

- Tokens live in the `tfc-vault` keychain under the service name `tfc-vault`,
  one item per profile, holding `{"host": …, "token": …}`.
- The environment variable name is derived from the hostname: `.` becomes `_`,
  `-` becomes `__`, and internationalised names are converted to punycode. So
  `tfe.example-corp.com` becomes `TF_TOKEN_tfe_example__corp_com`.
- When several profiles point at one host, the helper cannot choose between
  them. Name one with `TFC_VAULT_PROFILE`.
- A locked keychain answers a query with no results, which is indistinguishable
  from an absent token. tfc-vault checks the lock state before believing that,
  so Terraform is never told there are no credentials when there are.
- Reaching the keychain needs cgo. A `CGO_ENABLED=0` build is refused at compile
  time rather than producing a binary that fails at run time.

Verified against Terraform 1.11.3: both the environment variable injected by
`exec` and the token returned by the credentials helper are accepted for real
authentication.

日本語版は [README.ja.md](README.ja.md) にあります。

## Development

```sh
make build
make test
```

## Licence

MIT
