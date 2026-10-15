// Package scripts holds the tests of the shell scripts in .github/scripts, and of .editorconfig.
// The tests run each script in a temporary copy of the repository layout and serve every download
// from a local server, so they never reach the network: the tests of the scripts of the
// end-to-end jobs serve the Permit API from a fake, and run a stub docker. The .editorconfig test
// builds the editorconfig-checker that tools/go.mod pins, so its module must be in the module
// cache, as go -C tools mod download leaves it. The scan tests of gitleaks-range.sh run the
// gitleaks that tools/binaries.lock pins, which .github/scripts/fetch-tool.sh gitleaks installs
// as bin/gitleaks.
package scripts
