# Rendered with vhs: `vhs docs/demo.tape`
#
# HOME points at a throwaway directory, so the recording builds its own
# tfc-vault keychain and never touches the real one. Every step runs against an
# unlocked keychain, so macOS raises no passphrase dialog while recording.
Output docs/demo.gif

Set Shell "bash"
Set FontSize 18
Set Width 1180
Set Height 620
Set Padding 24
Set TypingSpeed 30ms

Set TypingSpeed 1ms
Hide
Type "export HOME=$(mktemp -d) PS1='$ ' PATH=$PWD/dist:$PATH CHECKPOINT_DISABLE=1" Enter
Type "mkdir -p $HOME/Library/Keychains" Enter
Type "clear" Enter
Show
Set TypingSpeed 30ms

Sleep 600ms
Type "# terraform login leaves your token in a plaintext file" Enter
Sleep 300ms
Type "# tfc-vault keeps it in a keychain that locks itself" Enter
Sleep 900ms

Type "tfc-vault add -no-verify   # demo token, so skip the check" Enter
Sleep 1600ms
Type "demo-token-not-a-real-one" Enter
Sleep 1300ms
Type "demo-passphrase" Enter
Sleep 700ms
Type "demo-passphrase" Enter
Sleep 1800ms

Type "tfc-vault list -v" Enter
Sleep 2000ms

Type "# the token reaches one command, nothing else" Enter
Sleep 300ms
Type "tfc-vault exec app.terraform.io -- terraform version" Enter
Sleep 2200ms

Type "tfc-vault lock" Enter
Sleep 1400ms
Type "# macOS asks for the passphrase from here on" Enter
Sleep 2200ms
