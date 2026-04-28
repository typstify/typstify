module looz.ws/typstify

go 1.25.5

require (
	gioui.org v0.9.1-0.20251215212054-7bcb315ee174
	gioui.org/x v0.9.0
	github.com/alecthomas/chroma/v2 v2.15.0
	github.com/apstndb/go-lsp-export v0.0.0-20250111110713-da502b65ef68
	github.com/denisbrodbeck/machineid v1.0.1
	github.com/dustin/go-humanize v1.0.1
	github.com/fsnotify/fsnotify v1.7.0
	github.com/gioui-plugins/gio-plugins v0.9.2
	github.com/google/go-cmp v0.6.0
	github.com/google/uuid v1.6.0
	github.com/inkeliz/giohyperlink v0.0.0-20220903215451-2ac5d54abdce
	github.com/inkeliz/giosvg v0.0.0-20240821232107-3208d4350d55
	github.com/mustafaturan/bus/v3 v3.0.3
	github.com/oligo/gioview v0.9.0
	github.com/oligo/gvcode v0.7.0
	github.com/pkg/errors v0.8.1
	github.com/rivo/uniseg v0.4.7
	github.com/sahilm/fuzzy v0.1.1
	github.com/saintfish/chardet v0.0.0-20230101081208-5e3ef4b5456d
	github.com/typstify/tpix-cli v0.8.5
	go.etcd.io/bbolt v1.3.11
	golang.org/x/exp/jsonrpc2 v0.0.0-20250911091902-df9299821621
	golang.org/x/exp/shiny v0.0.0-20250408133849-7e4ce0ab07d0
	golang.org/x/image v0.26.0
	golang.org/x/sys v0.40.0
	golang.org/x/telemetry v0.0.0-20260109210033-bd525da824e2
	golang.org/x/text v0.33.0
)

// use a local patch to fix the focus switching issue between the native webview and GioView
// Remove this if https://github.com/gioui/gio/pull/165 is accepted.
replace gioui.org => ../gio

require (
	gioui.org/shader v1.0.8 // indirect
	git.wow.st/gmp/jni v0.0.0-20210610011705-34026c7e22d0 // indirect
	github.com/apstndb/gotoolsdiff v0.29.0 // indirect
	github.com/bmatcuk/doublestar/v4 v4.10.0 // indirect
	github.com/dlclark/regexp2 v1.11.4 // indirect
	github.com/ebitengine/purego v0.8.0 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/go-text/typesetting v0.3.0 // indirect
	github.com/inkeliz/go_inkwasm v0.1.23-0.20240519174017-989fbe5b10f6 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/rdleal/intervalst v1.4.1 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/shirou/gopsutil/v4 v4.24.9 // indirect
	github.com/yuin/goldmark v1.4.13 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	golang.org/x/crypto v0.47.0 // indirect
	golang.org/x/exp v0.0.0-20250408133849-7e4ce0ab07d0 // indirect
	golang.org/x/exp/event v0.0.0-20250819193227-8b4c13bb791b // indirect
	golang.org/x/mod v0.33.0 // indirect
	golang.org/x/net v0.49.0 // indirect
	golang.org/x/xerrors v0.0.0-20240903120638-7835f813f4da // indirect
)
