# Manual rename protection

Auto Title preserves explicit tab names, including names present at startup.
The implementation is `internal/state/manual.go`.

## Ownership

The plugin compares the current label with its last applied label and desired label.
It can update a label that it applied before a restart.
It preserves other explicit labels, including names changed while the plugin stopped.
An empty label or the tab's position gives control back to Auto Title.
A label equal to the desired title remains automatic because the source cannot be distinguished.

The default label is the tab's position within its workspace, not its lifetime number.
The snapshot supplies tabs in display order.

Before each rename, a second snapshot checks the current label.
A concurrent layout rename, closed tab, or failed snapshot stops the write.
The check and write remain separate API calls. Atomic protection requires native conditional writes.

## State files

The default filename includes the SHA-256 hash of the absolute, canonical socket path.
Each Herdr server therefore has separate manual locks and applied labels.
The endpoint path remains stable across server restarts.
A missing socket setting keeps state in memory only.

`HERDR_AUTO_TITLE_MANUAL_FILE` overrides the default path.
Use a distinct override for each server. An empty override keeps state in memory only.

State contains `locked_tabs` and `applied_tabs`, keyed by tab ID.
Each value is the label that established ownership.
The plugin removes records for absent tabs or changed labels before each poll.
It writes through a temporary file, then renames that file into place.
Directories use mode 0700. Files use mode 0600.

## Upgrade behavior

The previous shared `manual-names.json` remains untouched.
The plugin does not import it because tab IDs can overlap across servers.
Existing explicit labels remain protected even without a state file.
Old generated labels without applied records also remain protected.
Clear such a label once to restore automatic naming.

To return any tab to Auto Title, clear its name through Herdr.
Editing a state file while the plugin runs does not update its memory.
