# Third-party notices

go-ios-vm ports code from, and vendors files built by, the projects below. See docs/port-inventory.md for the provenance of each component and internal/assets/lock.json for exact pins.

## vphone-cli

Porting reference: https://github.com/Lakr233/vphone-cli at b2804adb45a653ff3981f0705108975414dc8d5d. The licence below covers vphone-cli as a whole. Its MobileRestoreCore and MobileRecoveryCore directories are LGPL instead. go-ios-vm neither links nor executes them, and how its Go restore code may relate to them is an open decision recorded in docs/port-inventory.md.

```text
MIT License

Copyright (c) 2026 @Lakr233

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

## OpenAVB

`internal/assets/rom/OpenAVB/OpenAVB.bin` is built from https://github.com/owngoal-dev/OpenAVB at 65a776e9ae00b2e1de22b18cbbad1359945d7277. Its licence and the APSL-2.0 text are in `internal/assets/rom/OpenAVB/LICENSE` and `internal/assets/rom/OpenAVB/APSL-2.0.txt`.

## OpenSEP

`internal/assets/rom/OpenSEP/OpenSEP.bin` is built from https://github.com/owngoal-dev/OpenSEP at 8b1633f7e6606f0bc9e1422c93e8d2e056154d2f. Its licence is in `internal/assets/rom/OpenSEP/LICENSE`.

## go-bindings-macosplatform

https://github.com/deploymenttheory/go-bindings-macosplatform at d5b3d0ecb48ed106ddac46095bd91ec945b2833c, MIT; a Go module dependency.
