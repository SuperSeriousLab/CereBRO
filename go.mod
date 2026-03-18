module github.com/SuperSeriousLab/CereBRO

go 1.24.7

require (
	github.com/SuperSeriousLab/eidos-llm v0.0.0
	github.com/SuperSeriousLab/fugo v0.0.0-00010101000000-000000000000
	github.com/rs/zerolog v1.34.0
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.19 // indirect
	golang.org/x/sys v0.12.0 // indirect
)

replace github.com/SuperSeriousLab/eidos-llm => ../eidos-llm

replace github.com/SuperSeriousLab/fugo => ../fugo
