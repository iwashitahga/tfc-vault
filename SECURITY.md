# Security

tfc-vault handles Terraform API tokens, so please report suspected
vulnerabilities privately rather than in a public issue. Use GitHub's private
vulnerability reporting on this repository.

Please include what an attacker would gain and the steps to reproduce it.

## Scope

The README section "What this does and does not protect" states the intended
guarantees. Reports that fall outside them are still welcome, but the following
are known and documented rather than defects:

- A process running as you can read the token while the keychain is unlocked.
- `tfc-vault exec` places the token in the child process environment, where
  Terraform provider plugins and `local-exec` scripts can read it.
