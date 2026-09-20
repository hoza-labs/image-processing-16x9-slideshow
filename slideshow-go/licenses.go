package main

import _ "embed"

// LICENSE is a checked-in copy of the root license so plain go build works.
// The repository build script refreshes it before testing and compiling.
//
//go:embed LICENSE
var mitLicense string

//go:embed THIRD_PARTY_NOTICES.txt
var thirdPartyLicenses string

const sourceURL = "https://github.com/hoza-labs/image-processing-16x9-slideshow"
