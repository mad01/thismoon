package export

// The notices an exported file carries in its head comment for the
// third-party code inside it. THIRD-PARTY.md beside the service lists the
// same lines, and a test holds the two together.

// oflURL is the official text of the SIL Open Font License 1.1.
const oflURL = "https://openfontlicense.org/open-font-license-official-text/"

// fontCopyrights are the first lines of each family's OFL.txt in the
// google/fonts repository, one per family the cached fonts.css names.
var fontCopyrights = []string{
	"Copyright 2014-2020 The Fira Code Project Authors (https://github.com/tonsky/FiraCode)",
	"Copyright 2020 The Inter Project Authors (https://github.com/rsms/inter)",
	"Copyright 2018 The Lexend Project Authors (https://github.com/googlefonts/lexend), " +
		"with Reserved Font Name “RevReading Lexend”.",
	"Copyright 2019 The Work Sans Project Authors (https://github.com/weiweihuanghuang/Work-Sans)",
}

// prismLicense is the LICENSE file of the prismjs npm package, which
// webkit's bundle includes.
const prismLicense = `MIT LICENSE

Copyright (c) 2012 Lea Verou

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.`
