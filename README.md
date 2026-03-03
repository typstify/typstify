[![license](https://img.shields.io/badge/license-Apache%20V2-green)](https://github.com/typstify/typstify/blob/main/LICENSE)

<p align="center"><img src="version/appicon.png" width="100" /></p>

# Typstify

The cross-platform desktop editor for Typst. Unlock the power of Typst with Typstify. Get the professional power of LaTeX with a modern, intuitive editor designed for seamless typesetting and development.

## Source-Only Distribution

**Important:** The typstify project is distributed as source code only. For pre-compiled binary releases, please download from the [official website](https://typstify.com/download)




## Run

```sh
git clone https://github.com/typstify/typstify.git
cd typstify
go run .
```

## Build

This project uses [Gio](https://gioui.org/) to build the UI, to build a binary release, you have to install and use the gogio tool, please 
refer to [gio-cmd](https://git.sr.ht/~eliasnaur/gio-cmd) to learn more.


### Cross build

If you want to cross-compile on MacOS, install mingw64 first:

```sh
brew install mingw-w64
```

Then run the following command:

```sh
GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc CXX=x86_64-w64-mingw32-g++ make windows
```

The CC and CXX are needed by webview_go, Gio is not required to set them.

To compile in Windows, install MinGW-w64 first, follow the [guide](https://github.com/niXman/mingw-builds-binaries?tab=readme-ov-file) here, and then in git bash:

```sh
 export PATH="C:\Users\atzha\mingw64\bin:$PATH"
```


## Explore Further

-	[Official Website](https://typstify.com)

## License

This project is distributed under the [Apache License, Version 2.0](https://www.apache.org/licenses/LICENSE-2.0), see [LICENSE](./LICENSE) for more information.