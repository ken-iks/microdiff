# small-go

## GoCV / OpenCV Setup on macOS

If you encounter this error when building a gocv program:

```
/opt/homebrew/opt/opencv/include/opencv4/opencv2/core/cvdef.h:185:10: fatal error: 'limits' file not found
```

### Root Cause

Apple's Xcode Command Line Tools are not installed or not registered, so clang can't find C++ standard library headers.

Check with:
```bash
xcode-select -p
```

If this returns an error like `xcode-select: error: Unable to get active developer directory`, the tools aren't set up.

### Fix

**1. Install Xcode Command Line Tools:**

```bash
softwareupdate --list | grep -i "Command Line Tools"
sudo softwareupdate -i "Command Line Tools for Xcode-<version>.pkg"
```

**2. Point xcode-select to them:**

```bash
sudo xcode-select --switch /Library/Developer/CommandLineTools
xcode-select -p   # should now show /Library/Developer/CommandLineTools
```

**3. Remove any manual SDKROOT overrides:**

If you have this in `~/.zshrc`, comment it out:
```bash
# export SDKROOT=$(xcrun --sdk macosx --show-sdk-path)
```

Open a fresh shell and verify `echo "$SDKROOT"` is empty. Manual SDKROOT can point clang at an SDK missing full C++ headers.

**4. Verify C++ headers work:**

```bash
which clang        # expect /usr/bin/clang
clang --version    # should say Apple clang

cat << 'EOF' > test.cpp
#include <limits>
int main() { return 0; }
EOF

clang++ test.cpp -std=c++17
```

If this compiles, the standard library is correctly installed.

**5. Install OpenCV and run gocv:**

```bash
brew install opencv
```

Optionally expose it via PKG_CONFIG_PATH:
```bash
export PKG_CONFIG_PATH="/opt/homebrew/opt/opencv/lib/pkgconfig:$PKG_CONFIG_PATH"
```

Then build/run:
```bash
go get -u gocv.io/x/gocv
go run .
```

With CLT installed and active, the `'limits' file not found` error disappears and gocv builds successfully.

