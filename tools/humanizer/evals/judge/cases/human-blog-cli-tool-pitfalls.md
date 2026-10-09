---
id: human-blog-cli-tool-pitfalls
label: likely_human
bucket: human
split: test
source: github.com/mad01/mad01.github.io _posts/2015-03-21-command-line-tools-pitfalls.md, commit 36d412c5dead (2015-03-21)
license: owned by the repo author
generator: ""
words: 183
notes: First two paragraphs of a 2015 how-to post; lowercase sentence starts, typos (lest, corse, were for where) and an inline three-option enumeration.
---
Common pitfalls when using template files/support files in the working dir of the script, is easy to miss when you are writing command line tools. In this case using `python`. Some of the more annoying mistakes is when you have template files stores and referred to in a script, you assume that the person that will use you script stands in the folder were the script is. lest assume that we are using a file template.html in a folder named foobar. of corse this will not work if you are running the script from somewhere else. I normally stand in the same folder as the script when i am developing a tool. When you then stand in some other folder everything will fail since you are not giving the absolute path to the template files.

there is a few option here and it's to either only work if you standing in the correct folder, fix the paths to work anyway, or last a install script that creates the env you need and have a static path that can be used in any system.
