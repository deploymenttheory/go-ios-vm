# Machine store

`internal/machine` keeps each machine in its own directory in the library. The library defaults to `~/Library/Application Support/go-ios-vm/machines`; `--library` overrides it.

| Path | Contents |
| --- | --- |
| `machine.json` | Versioned configuration: name, native machine identifier bytes, firmware pair, hardware, and the machine's storage file names. |
| `disk.img`, `nvram.bin`, `sep-storage.bin`, `rom.bin`, `sep-rom.bin` | Storage that belongs to this machine. HOST-02 creates these files. |
| `stages/<stage>.json` | Receipt for a completed stage: its attempt and the size and SHA-256 of each evidence file. |
| `attempts/<id>.json` | One record per stage attempt: running, completed, failed or canceled, with the cause. |
| `native.json` | The last native state an owner observed, with its time. |
| `lock` | The OS lock file and the current holder's PID and purpose. |

## Guarantees

- **Identity is never regenerated.** A missing, corrupt, wrong-schema or mismatched `machine.json` is reported with how to recover. `Create` refuses an existing name.
- **Storage stays with its identity.** Storage files must be distinct plain file names inside the machine directory.
- **One owner at a time.** `Machine.Lock` takes an exclusive `flock` without waiting. A competing owner gets `ErrBusy`, naming the holder. The OS releases the lock when the owning process exits, even after `SIGKILL`. Every mutation requires the current `Owner`.
- **Writes are atomic.** Each write goes to a synced temporary file, is renamed into place, and the directory is synced. An interrupted write leaves the previous record, and the next owner removes the leftover.
- **A completed stage needs verified evidence.**
  - `Attempt.Complete` hashes the evidence itself.
  - A stage counts as completed only while its receipt parses and every evidence file still has its recorded size and digest.
  - A changed, missing or corrupt receipt is reported as a problem, never as completion.
  - Starting a stage withdraws its own receipt and every later stage's.
- **Attempts are separate from receipts.** Failed and canceled attempts keep their cause. A running attempt whose process died stays `running`, and `vm info` says it may have been interrupted. Neither produces a receipt.
- **Observations stay separate.** The native state is a timestamped past observation. Guest readiness is never stored, so `vm info` reports it as unknown.
