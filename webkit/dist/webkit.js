"use strict";
var Webkit = (() => {
  var __create = Object.create;
  var __defProp = Object.defineProperty;
  var __getOwnPropDesc = Object.getOwnPropertyDescriptor;
  var __getOwnPropNames = Object.getOwnPropertyNames;
  var __getProtoOf = Object.getPrototypeOf;
  var __hasOwnProp = Object.prototype.hasOwnProperty;
  var __commonJS = (cb, mod) => function __require() {
    return mod || (0, cb[__getOwnPropNames(cb)[0]])((mod = { exports: {} }).exports, mod), mod.exports;
  };
  var __export = (target, all) => {
    for (var name in all)
      __defProp(target, name, { get: all[name], enumerable: true });
  };
  var __copyProps = (to, from, except, desc) => {
    if (from && typeof from === "object" || typeof from === "function") {
      for (let key of __getOwnPropNames(from))
        if (!__hasOwnProp.call(to, key) && key !== except)
          __defProp(to, key, { get: () => from[key], enumerable: !(desc = __getOwnPropDesc(from, key)) || desc.enumerable });
    }
    return to;
  };
  var __toESM = (mod, isNodeMode, target) => (target = mod != null ? __create(__getProtoOf(mod)) : {}, __copyProps(
    // If the importer is in node compatibility mode or this is not an ESM
    // file that has been converted to a CommonJS file using a Babel-
    // compatible transform (i.e. "__esModule" has not been set), then set
    // "default" to the CommonJS "module.exports" for node compatibility.
    isNodeMode || !mod || !mod.__esModule ? __defProp(target, "default", { value: mod, enumerable: true }) : target,
    mod
  ));
  var __toCommonJS = (mod) => __copyProps(__defProp({}, "__esModule", { value: true }), mod);

  // node_modules/prismjs/prism.js
  var require_prism = __commonJS({
    "node_modules/prismjs/prism.js"(exports, module) {
      var _self = typeof window !== "undefined" ? window : typeof WorkerGlobalScope !== "undefined" && self instanceof WorkerGlobalScope ? self : {};
      var Prism3 = (function(_self2) {
        var lang = /(?:^|\s)lang(?:uage)?-([\w-]+)(?=\s|$)/i;
        var uniqueId = 0;
        var plainTextGrammar = {};
        var _ = {
          /**
           * By default, Prism will attempt to highlight all code elements (by calling {@link Prism.highlightAll}) on the
           * current page after the page finished loading. This might be a problem if e.g. you wanted to asynchronously load
           * additional languages or plugins yourself.
           *
           * By setting this value to `true`, Prism will not automatically highlight all code elements on the page.
           *
           * You obviously have to change this value before the automatic highlighting started. To do this, you can add an
           * empty Prism object into the global scope before loading the Prism script like this:
           *
           * ```js
           * window.Prism = window.Prism || {};
           * Prism.manual = true;
           * // add a new <script> to load Prism's script
           * ```
           *
           * @default false
           * @type {boolean}
           * @memberof Prism
           * @public
           */
          manual: _self2.Prism && _self2.Prism.manual,
          /**
           * By default, if Prism is in a web worker, it assumes that it is in a worker it created itself, so it uses
           * `addEventListener` to communicate with its parent instance. However, if you're using Prism manually in your
           * own worker, you don't want it to do this.
           *
           * By setting this value to `true`, Prism will not add its own listeners to the worker.
           *
           * You obviously have to change this value before Prism executes. To do this, you can add an
           * empty Prism object into the global scope before loading the Prism script like this:
           *
           * ```js
           * window.Prism = window.Prism || {};
           * Prism.disableWorkerMessageHandler = true;
           * // Load Prism's script
           * ```
           *
           * @default false
           * @type {boolean}
           * @memberof Prism
           * @public
           */
          disableWorkerMessageHandler: _self2.Prism && _self2.Prism.disableWorkerMessageHandler,
          /**
           * A namespace for utility methods.
           *
           * All function in this namespace that are not explicitly marked as _public_ are for __internal use only__ and may
           * change or disappear at any time.
           *
           * @namespace
           * @memberof Prism
           */
          util: {
            encode: function encode(tokens) {
              if (tokens instanceof Token) {
                return new Token(tokens.type, encode(tokens.content), tokens.alias);
              } else if (Array.isArray(tokens)) {
                return tokens.map(encode);
              } else {
                return tokens.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/\u00a0/g, " ");
              }
            },
            /**
             * Returns the name of the type of the given value.
             *
             * @param {any} o
             * @returns {string}
             * @example
             * type(null)      === 'Null'
             * type(undefined) === 'Undefined'
             * type(123)       === 'Number'
             * type('foo')     === 'String'
             * type(true)      === 'Boolean'
             * type([1, 2])    === 'Array'
             * type({})        === 'Object'
             * type(String)    === 'Function'
             * type(/abc+/)    === 'RegExp'
             */
            type: function(o) {
              return Object.prototype.toString.call(o).slice(8, -1);
            },
            /**
             * Returns a unique number for the given object. Later calls will still return the same number.
             *
             * @param {Object} obj
             * @returns {number}
             */
            objId: function(obj) {
              if (!obj["__id"]) {
                Object.defineProperty(obj, "__id", { value: ++uniqueId });
              }
              return obj["__id"];
            },
            /**
             * Creates a deep clone of the given object.
             *
             * The main intended use of this function is to clone language definitions.
             *
             * @param {T} o
             * @param {Record<number, any>} [visited]
             * @returns {T}
             * @template T
             */
            clone: function deepClone(o, visited) {
              visited = visited || {};
              var clone;
              var id;
              switch (_.util.type(o)) {
                case "Object":
                  id = _.util.objId(o);
                  if (visited[id]) {
                    return visited[id];
                  }
                  clone = /** @type {Record<string, any>} */
                  {};
                  visited[id] = clone;
                  for (var key in o) {
                    if (o.hasOwnProperty(key)) {
                      clone[key] = deepClone(o[key], visited);
                    }
                  }
                  return (
                    /** @type {any} */
                    clone
                  );
                case "Array":
                  id = _.util.objId(o);
                  if (visited[id]) {
                    return visited[id];
                  }
                  clone = [];
                  visited[id] = clone;
                  /** @type {Array} */
                  /** @type {any} */
                  o.forEach(function(v, i) {
                    clone[i] = deepClone(v, visited);
                  });
                  return (
                    /** @type {any} */
                    clone
                  );
                default:
                  return o;
              }
            },
            /**
             * Returns the Prism language of the given element set by a `language-xxxx` or `lang-xxxx` class.
             *
             * If no language is set for the element or the element is `null` or `undefined`, `none` will be returned.
             *
             * @param {Element} element
             * @returns {string}
             */
            getLanguage: function(element) {
              while (element) {
                var m = lang.exec(element.className);
                if (m) {
                  return m[1].toLowerCase();
                }
                element = element.parentElement;
              }
              return "none";
            },
            /**
             * Sets the Prism `language-xxxx` class of the given element.
             *
             * @param {Element} element
             * @param {string} language
             * @returns {void}
             */
            setLanguage: function(element, language) {
              element.className = element.className.replace(RegExp(lang, "gi"), "");
              element.classList.add("language-" + language);
            },
            /**
             * Returns the script element that is currently executing.
             *
             * This does __not__ work for line script element.
             *
             * @returns {HTMLScriptElement | null}
             */
            currentScript: function() {
              if (typeof document === "undefined") {
                return null;
              }
              if (document.currentScript && document.currentScript.tagName === "SCRIPT" && 1 < 2) {
                return (
                  /** @type {any} */
                  document.currentScript
                );
              }
              try {
                throw new Error();
              } catch (err) {
                var src = (/at [^(\r\n]*\((.*):[^:]+:[^:]+\)$/i.exec(err.stack) || [])[1];
                if (src) {
                  var scripts = document.getElementsByTagName("script");
                  for (var i in scripts) {
                    if (scripts[i].src == src) {
                      return scripts[i];
                    }
                  }
                }
                return null;
              }
            },
            /**
             * Returns whether a given class is active for `element`.
             *
             * The class can be activated if `element` or one of its ancestors has the given class and it can be deactivated
             * if `element` or one of its ancestors has the negated version of the given class. The _negated version_ of the
             * given class is just the given class with a `no-` prefix.
             *
             * Whether the class is active is determined by the closest ancestor of `element` (where `element` itself is
             * closest ancestor) that has the given class or the negated version of it. If neither `element` nor any of its
             * ancestors have the given class or the negated version of it, then the default activation will be returned.
             *
             * In the paradoxical situation where the closest ancestor contains __both__ the given class and the negated
             * version of it, the class is considered active.
             *
             * @param {Element} element
             * @param {string} className
             * @param {boolean} [defaultActivation=false]
             * @returns {boolean}
             */
            isActive: function(element, className, defaultActivation) {
              var no = "no-" + className;
              while (element) {
                var classList = element.classList;
                if (classList.contains(className)) {
                  return true;
                }
                if (classList.contains(no)) {
                  return false;
                }
                element = element.parentElement;
              }
              return !!defaultActivation;
            }
          },
          /**
           * This namespace contains all currently loaded languages and the some helper functions to create and modify languages.
           *
           * @namespace
           * @memberof Prism
           * @public
           */
          languages: {
            /**
             * The grammar for plain, unformatted text.
             */
            plain: plainTextGrammar,
            plaintext: plainTextGrammar,
            text: plainTextGrammar,
            txt: plainTextGrammar,
            /**
             * Creates a deep copy of the language with the given id and appends the given tokens.
             *
             * If a token in `redef` also appears in the copied language, then the existing token in the copied language
             * will be overwritten at its original position.
             *
             * ## Best practices
             *
             * Since the position of overwriting tokens (token in `redef` that overwrite tokens in the copied language)
             * doesn't matter, they can technically be in any order. However, this can be confusing to others that trying to
             * understand the language definition because, normally, the order of tokens matters in Prism grammars.
             *
             * Therefore, it is encouraged to order overwriting tokens according to the positions of the overwritten tokens.
             * Furthermore, all non-overwriting tokens should be placed after the overwriting ones.
             *
             * @param {string} id The id of the language to extend. This has to be a key in `Prism.languages`.
             * @param {Grammar} redef The new tokens to append.
             * @returns {Grammar} The new language created.
             * @public
             * @example
             * Prism.languages['css-with-colors'] = Prism.languages.extend('css', {
             *     // Prism.languages.css already has a 'comment' token, so this token will overwrite CSS' 'comment' token
             *     // at its original position
             *     'comment': { ... },
             *     // CSS doesn't have a 'color' token, so this token will be appended
             *     'color': /\b(?:red|green|blue)\b/
             * });
             */
            extend: function(id, redef) {
              var lang2 = _.util.clone(_.languages[id]);
              for (var key in redef) {
                lang2[key] = redef[key];
              }
              return lang2;
            },
            /**
             * Inserts tokens _before_ another token in a language definition or any other grammar.
             *
             * ## Usage
             *
             * This helper method makes it easy to modify existing languages. For example, the CSS language definition
             * not only defines CSS highlighting for CSS documents, but also needs to define highlighting for CSS embedded
             * in HTML through `<style>` elements. To do this, it needs to modify `Prism.languages.markup` and add the
             * appropriate tokens. However, `Prism.languages.markup` is a regular JavaScript object literal, so if you do
             * this:
             *
             * ```js
             * Prism.languages.markup.style = {
             *     // token
             * };
             * ```
             *
             * then the `style` token will be added (and processed) at the end. `insertBefore` allows you to insert tokens
             * before existing tokens. For the CSS example above, you would use it like this:
             *
             * ```js
             * Prism.languages.insertBefore('markup', 'cdata', {
             *     'style': {
             *         // token
             *     }
             * });
             * ```
             *
             * ## Special cases
             *
             * If the grammars of `inside` and `insert` have tokens with the same name, the tokens in `inside`'s grammar
             * will be ignored.
             *
             * This behavior can be used to insert tokens after `before`:
             *
             * ```js
             * Prism.languages.insertBefore('markup', 'comment', {
             *     'comment': Prism.languages.markup.comment,
             *     // tokens after 'comment'
             * });
             * ```
             *
             * ## Limitations
             *
             * The main problem `insertBefore` has to solve is iteration order. Since ES2015, the iteration order for object
             * properties is guaranteed to be the insertion order (except for integer keys) but some browsers behave
             * differently when keys are deleted and re-inserted. So `insertBefore` can't be implemented by temporarily
             * deleting properties which is necessary to insert at arbitrary positions.
             *
             * To solve this problem, `insertBefore` doesn't actually insert the given tokens into the target object.
             * Instead, it will create a new object and replace all references to the target object with the new one. This
             * can be done without temporarily deleting properties, so the iteration order is well-defined.
             *
             * However, only references that can be reached from `Prism.languages` or `insert` will be replaced. I.e. if
             * you hold the target object in a variable, then the value of the variable will not change.
             *
             * ```js
             * var oldMarkup = Prism.languages.markup;
             * var newMarkup = Prism.languages.insertBefore('markup', 'comment', { ... });
             *
             * assert(oldMarkup !== Prism.languages.markup);
             * assert(newMarkup === Prism.languages.markup);
             * ```
             *
             * @param {string} inside The property of `root` (e.g. a language id in `Prism.languages`) that contains the
             * object to be modified.
             * @param {string} before The key to insert before.
             * @param {Grammar} insert An object containing the key-value pairs to be inserted.
             * @param {Object<string, any>} [root] The object containing `inside`, i.e. the object that contains the
             * object to be modified.
             *
             * Defaults to `Prism.languages`.
             * @returns {Grammar} The new grammar object.
             * @public
             */
            insertBefore: function(inside, before, insert, root) {
              root = root || /** @type {any} */
              _.languages;
              var grammar = root[inside];
              var ret = {};
              for (var token in grammar) {
                if (grammar.hasOwnProperty(token)) {
                  if (token == before) {
                    for (var newToken in insert) {
                      if (insert.hasOwnProperty(newToken)) {
                        ret[newToken] = insert[newToken];
                      }
                    }
                  }
                  if (!insert.hasOwnProperty(token)) {
                    ret[token] = grammar[token];
                  }
                }
              }
              var old = root[inside];
              root[inside] = ret;
              _.languages.DFS(_.languages, function(key, value) {
                if (value === old && key != inside) {
                  this[key] = ret;
                }
              });
              return ret;
            },
            // Traverse a language definition with Depth First Search
            DFS: function DFS(o, callback, type, visited) {
              visited = visited || {};
              var objId = _.util.objId;
              for (var i in o) {
                if (o.hasOwnProperty(i)) {
                  callback.call(o, i, o[i], type || i);
                  var property = o[i];
                  var propertyType = _.util.type(property);
                  if (propertyType === "Object" && !visited[objId(property)]) {
                    visited[objId(property)] = true;
                    DFS(property, callback, null, visited);
                  } else if (propertyType === "Array" && !visited[objId(property)]) {
                    visited[objId(property)] = true;
                    DFS(property, callback, i, visited);
                  }
                }
              }
            }
          },
          plugins: {},
          /**
           * This is the most high-level function in Prism’s API.
           * It fetches all the elements that have a `.language-xxxx` class and then calls {@link Prism.highlightElement} on
           * each one of them.
           *
           * This is equivalent to `Prism.highlightAllUnder(document, async, callback)`.
           *
           * @param {boolean} [async=false] Same as in {@link Prism.highlightAllUnder}.
           * @param {HighlightCallback} [callback] Same as in {@link Prism.highlightAllUnder}.
           * @memberof Prism
           * @public
           */
          highlightAll: function(async, callback) {
            _.highlightAllUnder(document, async, callback);
          },
          /**
           * Fetches all the descendants of `container` that have a `.language-xxxx` class and then calls
           * {@link Prism.highlightElement} on each one of them.
           *
           * The following hooks will be run:
           * 1. `before-highlightall`
           * 2. `before-all-elements-highlight`
           * 3. All hooks of {@link Prism.highlightElement} for each element.
           *
           * @param {ParentNode} container The root element, whose descendants that have a `.language-xxxx` class will be highlighted.
           * @param {boolean} [async=false] Whether each element is to be highlighted asynchronously using Web Workers.
           * @param {HighlightCallback} [callback] An optional callback to be invoked on each element after its highlighting is done.
           * @memberof Prism
           * @public
           */
          highlightAllUnder: function(container, async, callback) {
            var env = {
              callback,
              container,
              selector: 'code[class*="language-"], [class*="language-"] code, code[class*="lang-"], [class*="lang-"] code'
            };
            _.hooks.run("before-highlightall", env);
            env.elements = Array.prototype.slice.apply(env.container.querySelectorAll(env.selector));
            _.hooks.run("before-all-elements-highlight", env);
            for (var i = 0, element; element = env.elements[i++]; ) {
              _.highlightElement(element, async === true, env.callback);
            }
          },
          /**
           * Highlights the code inside a single element.
           *
           * The following hooks will be run:
           * 1. `before-sanity-check`
           * 2. `before-highlight`
           * 3. All hooks of {@link Prism.highlight}. These hooks will be run by an asynchronous worker if `async` is `true`.
           * 4. `before-insert`
           * 5. `after-highlight`
           * 6. `complete`
           *
           * Some the above hooks will be skipped if the element doesn't contain any text or there is no grammar loaded for
           * the element's language.
           *
           * @param {Element} element The element containing the code.
           * It must have a class of `language-xxxx` to be processed, where `xxxx` is a valid language identifier.
           * @param {boolean} [async=false] Whether the element is to be highlighted asynchronously using Web Workers
           * to improve performance and avoid blocking the UI when highlighting very large chunks of code. This option is
           * [disabled by default](https://prismjs.com/faq.html#why-is-asynchronous-highlighting-disabled-by-default).
           *
           * Note: All language definitions required to highlight the code must be included in the main `prism.js` file for
           * asynchronous highlighting to work. You can build your own bundle on the
           * [Download page](https://prismjs.com/download.html).
           * @param {HighlightCallback} [callback] An optional callback to be invoked after the highlighting is done.
           * Mostly useful when `async` is `true`, since in that case, the highlighting is done asynchronously.
           * @memberof Prism
           * @public
           */
          highlightElement: function(element, async, callback) {
            var language = _.util.getLanguage(element);
            var grammar = _.languages[language];
            _.util.setLanguage(element, language);
            var parent = element.parentElement;
            if (parent && parent.nodeName.toLowerCase() === "pre") {
              _.util.setLanguage(parent, language);
            }
            var code = element.textContent;
            var env = {
              element,
              language,
              grammar,
              code
            };
            function insertHighlightedCode(highlightedCode) {
              env.highlightedCode = highlightedCode;
              _.hooks.run("before-insert", env);
              env.element.innerHTML = env.highlightedCode;
              _.hooks.run("after-highlight", env);
              _.hooks.run("complete", env);
              callback && callback.call(env.element);
            }
            _.hooks.run("before-sanity-check", env);
            parent = env.element.parentElement;
            if (parent && parent.nodeName.toLowerCase() === "pre" && !parent.hasAttribute("tabindex")) {
              parent.setAttribute("tabindex", "0");
            }
            if (!env.code) {
              _.hooks.run("complete", env);
              callback && callback.call(env.element);
              return;
            }
            _.hooks.run("before-highlight", env);
            if (!env.grammar) {
              insertHighlightedCode(_.util.encode(env.code));
              return;
            }
            if (async && _self2.Worker) {
              var worker = new Worker(_.filename);
              worker.onmessage = function(evt) {
                insertHighlightedCode(evt.data);
              };
              worker.postMessage(JSON.stringify({
                language: env.language,
                code: env.code,
                immediateClose: true
              }));
            } else {
              insertHighlightedCode(_.highlight(env.code, env.grammar, env.language));
            }
          },
          /**
           * Low-level function, only use if you know what you’re doing. It accepts a string of text as input
           * and the language definitions to use, and returns a string with the HTML produced.
           *
           * The following hooks will be run:
           * 1. `before-tokenize`
           * 2. `after-tokenize`
           * 3. `wrap`: On each {@link Token}.
           *
           * @param {string} text A string with the code to be highlighted.
           * @param {Grammar} grammar An object containing the tokens to use.
           *
           * Usually a language definition like `Prism.languages.markup`.
           * @param {string} language The name of the language definition passed to `grammar`.
           * @returns {string} The highlighted HTML.
           * @memberof Prism
           * @public
           * @example
           * Prism.highlight('var foo = true;', Prism.languages.javascript, 'javascript');
           */
          highlight: function(text, grammar, language) {
            var env = {
              code: text,
              grammar,
              language
            };
            _.hooks.run("before-tokenize", env);
            if (!env.grammar) {
              throw new Error('The language "' + env.language + '" has no grammar.');
            }
            env.tokens = _.tokenize(env.code, env.grammar);
            _.hooks.run("after-tokenize", env);
            return Token.stringify(_.util.encode(env.tokens), env.language);
          },
          /**
           * This is the heart of Prism, and the most low-level function you can use. It accepts a string of text as input
           * and the language definitions to use, and returns an array with the tokenized code.
           *
           * When the language definition includes nested tokens, the function is called recursively on each of these tokens.
           *
           * This method could be useful in other contexts as well, as a very crude parser.
           *
           * @param {string} text A string with the code to be highlighted.
           * @param {Grammar} grammar An object containing the tokens to use.
           *
           * Usually a language definition like `Prism.languages.markup`.
           * @returns {TokenStream} An array of strings and tokens, a token stream.
           * @memberof Prism
           * @public
           * @example
           * let code = `var foo = 0;`;
           * let tokens = Prism.tokenize(code, Prism.languages.javascript);
           * tokens.forEach(token => {
           *     if (token instanceof Prism.Token && token.type === 'number') {
           *         console.log(`Found numeric literal: ${token.content}`);
           *     }
           * });
           */
          tokenize: function(text, grammar) {
            var rest = grammar.rest;
            if (rest) {
              for (var token in rest) {
                grammar[token] = rest[token];
              }
              delete grammar.rest;
            }
            var tokenList = new LinkedList();
            addAfter(tokenList, tokenList.head, text);
            matchGrammar(text, tokenList, grammar, tokenList.head, 0);
            return toArray(tokenList);
          },
          /**
           * @namespace
           * @memberof Prism
           * @public
           */
          hooks: {
            all: {},
            /**
             * Adds the given callback to the list of callbacks for the given hook.
             *
             * The callback will be invoked when the hook it is registered for is run.
             * Hooks are usually directly run by a highlight function but you can also run hooks yourself.
             *
             * One callback function can be registered to multiple hooks and the same hook multiple times.
             *
             * @param {string} name The name of the hook.
             * @param {HookCallback} callback The callback function which is given environment variables.
             * @public
             */
            add: function(name, callback) {
              var hooks = _.hooks.all;
              hooks[name] = hooks[name] || [];
              hooks[name].push(callback);
            },
            /**
             * Runs a hook invoking all registered callbacks with the given environment variables.
             *
             * Callbacks will be invoked synchronously and in the order in which they were registered.
             *
             * @param {string} name The name of the hook.
             * @param {Object<string, any>} env The environment variables of the hook passed to all callbacks registered.
             * @public
             */
            run: function(name, env) {
              var callbacks = _.hooks.all[name];
              if (!callbacks || !callbacks.length) {
                return;
              }
              for (var i = 0, callback; callback = callbacks[i++]; ) {
                callback(env);
              }
            }
          },
          Token
        };
        _self2.Prism = _;
        function Token(type, content, alias, matchedStr) {
          this.type = type;
          this.content = content;
          this.alias = alias;
          this.length = (matchedStr || "").length | 0;
        }
        Token.stringify = function stringify(o, language) {
          if (typeof o == "string") {
            return o;
          }
          if (Array.isArray(o)) {
            var s = "";
            o.forEach(function(e) {
              s += stringify(e, language);
            });
            return s;
          }
          var env = {
            type: o.type,
            content: stringify(o.content, language),
            tag: "span",
            classes: ["token", o.type],
            attributes: {},
            language
          };
          var aliases = o.alias;
          if (aliases) {
            if (Array.isArray(aliases)) {
              Array.prototype.push.apply(env.classes, aliases);
            } else {
              env.classes.push(aliases);
            }
          }
          _.hooks.run("wrap", env);
          var attributes = "";
          for (var name in env.attributes) {
            attributes += " " + name + '="' + (env.attributes[name] || "").replace(/"/g, "&quot;") + '"';
          }
          return "<" + env.tag + ' class="' + env.classes.join(" ") + '"' + attributes + ">" + env.content + "</" + env.tag + ">";
        };
        function matchPattern(pattern, pos, text, lookbehind) {
          pattern.lastIndex = pos;
          var match = pattern.exec(text);
          if (match && lookbehind && match[1]) {
            var lookbehindLength = match[1].length;
            match.index += lookbehindLength;
            match[0] = match[0].slice(lookbehindLength);
          }
          return match;
        }
        function matchGrammar(text, tokenList, grammar, startNode, startPos, rematch) {
          for (var token in grammar) {
            if (!grammar.hasOwnProperty(token) || !grammar[token]) {
              continue;
            }
            var patterns = grammar[token];
            patterns = Array.isArray(patterns) ? patterns : [patterns];
            for (var j = 0; j < patterns.length; ++j) {
              if (rematch && rematch.cause == token + "," + j) {
                return;
              }
              var patternObj = patterns[j];
              var inside = patternObj.inside;
              var lookbehind = !!patternObj.lookbehind;
              var greedy = !!patternObj.greedy;
              var alias = patternObj.alias;
              if (greedy && !patternObj.pattern.global) {
                var flags = patternObj.pattern.toString().match(/[imsuy]*$/)[0];
                patternObj.pattern = RegExp(patternObj.pattern.source, flags + "g");
              }
              var pattern = patternObj.pattern || patternObj;
              for (var currentNode = startNode.next, pos = startPos; currentNode !== tokenList.tail; pos += currentNode.value.length, currentNode = currentNode.next) {
                if (rematch && pos >= rematch.reach) {
                  break;
                }
                var str = currentNode.value;
                if (tokenList.length > text.length) {
                  return;
                }
                if (str instanceof Token) {
                  continue;
                }
                var removeCount = 1;
                var match;
                if (greedy) {
                  match = matchPattern(pattern, pos, text, lookbehind);
                  if (!match || match.index >= text.length) {
                    break;
                  }
                  var from = match.index;
                  var to = match.index + match[0].length;
                  var p = pos;
                  p += currentNode.value.length;
                  while (from >= p) {
                    currentNode = currentNode.next;
                    p += currentNode.value.length;
                  }
                  p -= currentNode.value.length;
                  pos = p;
                  if (currentNode.value instanceof Token) {
                    continue;
                  }
                  for (var k = currentNode; k !== tokenList.tail && (p < to || typeof k.value === "string"); k = k.next) {
                    removeCount++;
                    p += k.value.length;
                  }
                  removeCount--;
                  str = text.slice(pos, p);
                  match.index -= pos;
                } else {
                  match = matchPattern(pattern, 0, str, lookbehind);
                  if (!match) {
                    continue;
                  }
                }
                var from = match.index;
                var matchStr = match[0];
                var before = str.slice(0, from);
                var after = str.slice(from + matchStr.length);
                var reach = pos + str.length;
                if (rematch && reach > rematch.reach) {
                  rematch.reach = reach;
                }
                var removeFrom = currentNode.prev;
                if (before) {
                  removeFrom = addAfter(tokenList, removeFrom, before);
                  pos += before.length;
                }
                removeRange(tokenList, removeFrom, removeCount);
                var wrapped = new Token(token, inside ? _.tokenize(matchStr, inside) : matchStr, alias, matchStr);
                currentNode = addAfter(tokenList, removeFrom, wrapped);
                if (after) {
                  addAfter(tokenList, currentNode, after);
                }
                if (removeCount > 1) {
                  var nestedRematch = {
                    cause: token + "," + j,
                    reach
                  };
                  matchGrammar(text, tokenList, grammar, currentNode.prev, pos, nestedRematch);
                  if (rematch && nestedRematch.reach > rematch.reach) {
                    rematch.reach = nestedRematch.reach;
                  }
                }
              }
            }
          }
        }
        function LinkedList() {
          var head = { value: null, prev: null, next: null };
          var tail = { value: null, prev: head, next: null };
          head.next = tail;
          this.head = head;
          this.tail = tail;
          this.length = 0;
        }
        function addAfter(list, node, value) {
          var next = node.next;
          var newNode = { value, prev: node, next };
          node.next = newNode;
          next.prev = newNode;
          list.length++;
          return newNode;
        }
        function removeRange(list, node, count) {
          var next = node.next;
          for (var i = 0; i < count && next !== list.tail; i++) {
            next = next.next;
          }
          node.next = next;
          next.prev = node;
          list.length -= i;
        }
        function toArray(list) {
          var array = [];
          var node = list.head.next;
          while (node !== list.tail) {
            array.push(node.value);
            node = node.next;
          }
          return array;
        }
        if (!_self2.document) {
          if (!_self2.addEventListener) {
            return _;
          }
          if (!_.disableWorkerMessageHandler) {
            _self2.addEventListener("message", function(evt) {
              var message = JSON.parse(evt.data);
              var lang2 = message.language;
              var code = message.code;
              var immediateClose = message.immediateClose;
              _self2.postMessage(_.highlight(code, _.languages[lang2], lang2));
              if (immediateClose) {
                _self2.close();
              }
            }, false);
          }
          return _;
        }
        var script = _.util.currentScript();
        if (script) {
          _.filename = script.src;
          if (script.hasAttribute("data-manual")) {
            _.manual = true;
          }
        }
        function highlightAutomaticallyCallback() {
          if (!_.manual) {
            _.highlightAll();
          }
        }
        if (!_.manual) {
          var readyState = document.readyState;
          if (readyState === "loading" || readyState === "interactive" && script && script.defer) {
            document.addEventListener("DOMContentLoaded", highlightAutomaticallyCallback);
          } else {
            if (window.requestAnimationFrame) {
              window.requestAnimationFrame(highlightAutomaticallyCallback);
            } else {
              window.setTimeout(highlightAutomaticallyCallback, 16);
            }
          }
        }
        return _;
      })(_self);
      if (typeof module !== "undefined" && module.exports) {
        module.exports = Prism3;
      }
      if (typeof global !== "undefined") {
        global.Prism = Prism3;
      }
      Prism3.languages.markup = {
        "comment": {
          pattern: /<!--(?:(?!<!--)[\s\S])*?-->/,
          greedy: true
        },
        "prolog": {
          pattern: /<\?[\s\S]+?\?>/,
          greedy: true
        },
        "doctype": {
          // https://www.w3.org/TR/xml/#NT-doctypedecl
          pattern: /<!DOCTYPE(?:[^>"'[\]]|"[^"]*"|'[^']*')+(?:\[(?:[^<"'\]]|"[^"]*"|'[^']*'|<(?!!--)|<!--(?:[^-]|-(?!->))*-->)*\]\s*)?>/i,
          greedy: true,
          inside: {
            "internal-subset": {
              pattern: /(^[^\[]*\[)[\s\S]+(?=\]>$)/,
              lookbehind: true,
              greedy: true,
              inside: null
              // see below
            },
            "string": {
              pattern: /"[^"]*"|'[^']*'/,
              greedy: true
            },
            "punctuation": /^<!|>$|[[\]]/,
            "doctype-tag": /^DOCTYPE/i,
            "name": /[^\s<>'"]+/
          }
        },
        "cdata": {
          pattern: /<!\[CDATA\[[\s\S]*?\]\]>/i,
          greedy: true
        },
        "tag": {
          pattern: /<\/?(?!\d)[^\s>\/=$<%]+(?:\s(?:\s*[^\s>\/=]+(?:\s*=\s*(?:"[^"]*"|'[^']*'|[^\s'">=]+(?=[\s>]))|(?=[\s/>])))+)?\s*\/?>/,
          greedy: true,
          inside: {
            "tag": {
              pattern: /^<\/?[^\s>\/]+/,
              inside: {
                "punctuation": /^<\/?/,
                "namespace": /^[^\s>\/:]+:/
              }
            },
            "special-attr": [],
            "attr-value": {
              pattern: /=\s*(?:"[^"]*"|'[^']*'|[^\s'">=]+)/,
              inside: {
                "punctuation": [
                  {
                    pattern: /^=/,
                    alias: "attr-equals"
                  },
                  {
                    pattern: /^(\s*)["']|["']$/,
                    lookbehind: true
                  }
                ]
              }
            },
            "punctuation": /\/?>/,
            "attr-name": {
              pattern: /[^\s>\/]+/,
              inside: {
                "namespace": /^[^\s>\/:]+:/
              }
            }
          }
        },
        "entity": [
          {
            pattern: /&[\da-z]{1,8};/i,
            alias: "named-entity"
          },
          /&#x?[\da-f]{1,8};/i
        ]
      };
      Prism3.languages.markup["tag"].inside["attr-value"].inside["entity"] = Prism3.languages.markup["entity"];
      Prism3.languages.markup["doctype"].inside["internal-subset"].inside = Prism3.languages.markup;
      Prism3.hooks.add("wrap", function(env) {
        if (env.type === "entity") {
          env.attributes["title"] = env.content.replace(/&amp;/, "&");
        }
      });
      Object.defineProperty(Prism3.languages.markup.tag, "addInlined", {
        /**
         * Adds an inlined language to markup.
         *
         * An example of an inlined language is CSS with `<style>` tags.
         *
         * @param {string} tagName The name of the tag that contains the inlined language. This name will be treated as
         * case insensitive.
         * @param {string} lang The language key.
         * @example
         * addInlined('style', 'css');
         */
        value: function addInlined(tagName, lang) {
          var includedCdataInside = {};
          includedCdataInside["language-" + lang] = {
            pattern: /(^<!\[CDATA\[)[\s\S]+?(?=\]\]>$)/i,
            lookbehind: true,
            inside: Prism3.languages[lang]
          };
          includedCdataInside["cdata"] = /^<!\[CDATA\[|\]\]>$/i;
          var inside = {
            "included-cdata": {
              pattern: /<!\[CDATA\[[\s\S]*?\]\]>/i,
              inside: includedCdataInside
            }
          };
          inside["language-" + lang] = {
            pattern: /[\s\S]+/,
            inside: Prism3.languages[lang]
          };
          var def = {};
          def[tagName] = {
            pattern: RegExp(/(<__[^>]*>)(?:<!\[CDATA\[(?:[^\]]|\](?!\]>))*\]\]>|(?!<!\[CDATA\[)[\s\S])*?(?=<\/__>)/.source.replace(/__/g, function() {
              return tagName;
            }), "i"),
            lookbehind: true,
            greedy: true,
            inside
          };
          Prism3.languages.insertBefore("markup", "cdata", def);
        }
      });
      Object.defineProperty(Prism3.languages.markup.tag, "addAttribute", {
        /**
         * Adds an pattern to highlight languages embedded in HTML attributes.
         *
         * An example of an inlined language is CSS with `style` attributes.
         *
         * @param {string} attrName The name of the tag that contains the inlined language. This name will be treated as
         * case insensitive.
         * @param {string} lang The language key.
         * @example
         * addAttribute('style', 'css');
         */
        value: function(attrName, lang) {
          Prism3.languages.markup.tag.inside["special-attr"].push({
            pattern: RegExp(
              /(^|["'\s])/.source + "(?:" + attrName + ")" + /\s*=\s*(?:"[^"]*"|'[^']*'|[^\s'">=]+(?=[\s>]))/.source,
              "i"
            ),
            lookbehind: true,
            inside: {
              "attr-name": /^[^\s=]+/,
              "attr-value": {
                pattern: /=[\s\S]+/,
                inside: {
                  "value": {
                    pattern: /(^=\s*(["']|(?!["'])))\S[\s\S]*(?=\2$)/,
                    lookbehind: true,
                    alias: [lang, "language-" + lang],
                    inside: Prism3.languages[lang]
                  },
                  "punctuation": [
                    {
                      pattern: /^=/,
                      alias: "attr-equals"
                    },
                    /"|'/
                  ]
                }
              }
            }
          });
        }
      });
      Prism3.languages.html = Prism3.languages.markup;
      Prism3.languages.mathml = Prism3.languages.markup;
      Prism3.languages.svg = Prism3.languages.markup;
      Prism3.languages.xml = Prism3.languages.extend("markup", {});
      Prism3.languages.ssml = Prism3.languages.xml;
      Prism3.languages.atom = Prism3.languages.xml;
      Prism3.languages.rss = Prism3.languages.xml;
      (function(Prism4) {
        var string = /(?:"(?:\\(?:\r\n|[\s\S])|[^"\\\r\n])*"|'(?:\\(?:\r\n|[\s\S])|[^'\\\r\n])*')/;
        Prism4.languages.css = {
          "comment": /\/\*[\s\S]*?\*\//,
          "atrule": {
            pattern: RegExp("@[\\w-](?:" + /[^;{\s"']|\s+(?!\s)/.source + "|" + string.source + ")*?" + /(?:;|(?=\s*\{))/.source),
            inside: {
              "rule": /^@[\w-]+/,
              "selector-function-argument": {
                pattern: /(\bselector\s*\(\s*(?![\s)]))(?:[^()\s]|\s+(?![\s)])|\((?:[^()]|\([^()]*\))*\))+(?=\s*\))/,
                lookbehind: true,
                alias: "selector"
              },
              "keyword": {
                pattern: /(^|[^\w-])(?:and|not|only|or)(?![\w-])/,
                lookbehind: true
              }
              // See rest below
            }
          },
          "url": {
            // https://drafts.csswg.org/css-values-3/#urls
            pattern: RegExp("\\burl\\((?:" + string.source + "|" + /(?:[^\\\r\n()"']|\\[\s\S])*/.source + ")\\)", "i"),
            greedy: true,
            inside: {
              "function": /^url/i,
              "punctuation": /^\(|\)$/,
              "string": {
                pattern: RegExp("^" + string.source + "$"),
                alias: "url"
              }
            }
          },
          "selector": {
            pattern: RegExp(`(^|[{}\\s])[^{}\\s](?:[^{};"'\\s]|\\s+(?![\\s{])|` + string.source + ")*(?=\\s*\\{)"),
            lookbehind: true
          },
          "string": {
            pattern: string,
            greedy: true
          },
          "property": {
            pattern: /(^|[^-\w\xA0-\uFFFF])(?!\s)[-_a-z\xA0-\uFFFF](?:(?!\s)[-\w\xA0-\uFFFF])*(?=\s*:)/i,
            lookbehind: true
          },
          "important": /!important\b/i,
          "function": {
            pattern: /(^|[^-a-z0-9])[-a-z0-9]+(?=\()/i,
            lookbehind: true
          },
          "punctuation": /[(){};:,]/
        };
        Prism4.languages.css["atrule"].inside.rest = Prism4.languages.css;
        var markup = Prism4.languages.markup;
        if (markup) {
          markup.tag.addInlined("style", "css");
          markup.tag.addAttribute("style", "css");
        }
      })(Prism3);
      Prism3.languages.clike = {
        "comment": [
          {
            pattern: /(^|[^\\])\/\*[\s\S]*?(?:\*\/|$)/,
            lookbehind: true,
            greedy: true
          },
          {
            pattern: /(^|[^\\:])\/\/.*/,
            lookbehind: true,
            greedy: true
          }
        ],
        "string": {
          pattern: /(["'])(?:\\(?:\r\n|[\s\S])|(?!\1)[^\\\r\n])*\1/,
          greedy: true
        },
        "class-name": {
          pattern: /(\b(?:class|extends|implements|instanceof|interface|new|trait)\s+|\bcatch\s+\()[\w.\\]+/i,
          lookbehind: true,
          inside: {
            "punctuation": /[.\\]/
          }
        },
        "keyword": /\b(?:break|catch|continue|do|else|finally|for|function|if|in|instanceof|new|null|return|throw|try|while)\b/,
        "boolean": /\b(?:false|true)\b/,
        "function": /\b\w+(?=\()/,
        "number": /\b0x[\da-f]+\b|(?:\b\d+(?:\.\d*)?|\B\.\d+)(?:e[+-]?\d+)?/i,
        "operator": /[<>]=?|[!=]=?=?|--?|\+\+?|&&?|\|\|?|[?*/~^%]/,
        "punctuation": /[{}[\];(),.:]/
      };
      Prism3.languages.javascript = Prism3.languages.extend("clike", {
        "class-name": [
          Prism3.languages.clike["class-name"],
          {
            pattern: /(^|[^$\w\xA0-\uFFFF])(?!\s)[_$A-Z\xA0-\uFFFF](?:(?!\s)[$\w\xA0-\uFFFF])*(?=\.(?:constructor|prototype))/,
            lookbehind: true
          }
        ],
        "keyword": [
          {
            pattern: /((?:^|\})\s*)catch\b/,
            lookbehind: true
          },
          {
            pattern: /(^|[^.]|\.\.\.\s*)\b(?:as|assert(?=\s*\{)|async(?=\s*(?:function\b|\(|[$\w\xA0-\uFFFF]|$))|await|break|case|class|const|continue|debugger|default|delete|do|else|enum|export|extends|finally(?=\s*(?:\{|$))|for|from(?=\s*(?:['"]|$))|function|(?:get|set)(?=\s*(?:[#\[$\w\xA0-\uFFFF]|$))|if|implements|import|in|instanceof|interface|let|new|null|of|package|private|protected|public|return|static|super|switch|this|throw|try|typeof|undefined|var|void|while|with|yield)\b/,
            lookbehind: true
          }
        ],
        // Allow for all non-ASCII characters (See http://stackoverflow.com/a/2008444)
        "function": /#?(?!\s)[_$a-zA-Z\xA0-\uFFFF](?:(?!\s)[$\w\xA0-\uFFFF])*(?=\s*(?:\.\s*(?:apply|bind|call)\s*)?\()/,
        "number": {
          pattern: RegExp(
            /(^|[^\w$])/.source + "(?:" + // constant
            (/NaN|Infinity/.source + "|" + // binary integer
            /0[bB][01]+(?:_[01]+)*n?/.source + "|" + // octal integer
            /0[oO][0-7]+(?:_[0-7]+)*n?/.source + "|" + // hexadecimal integer
            /0[xX][\dA-Fa-f]+(?:_[\dA-Fa-f]+)*n?/.source + "|" + // decimal bigint
            /\d+(?:_\d+)*n/.source + "|" + // decimal number (integer or float) but no bigint
            /(?:\d+(?:_\d+)*(?:\.(?:\d+(?:_\d+)*)?)?|\.\d+(?:_\d+)*)(?:[Ee][+-]?\d+(?:_\d+)*)?/.source) + ")" + /(?![\w$])/.source
          ),
          lookbehind: true
        },
        "operator": /--|\+\+|\*\*=?|=>|&&=?|\|\|=?|[!=]==|<<=?|>>>?=?|[-+*/%&|^!=<>]=?|\.{3}|\?\?=?|\?\.?|[~:]/
      });
      Prism3.languages.javascript["class-name"][0].pattern = /(\b(?:class|extends|implements|instanceof|interface|new)\s+)[\w.\\]+/;
      Prism3.languages.insertBefore("javascript", "keyword", {
        "regex": {
          pattern: RegExp(
            // lookbehind
            // eslint-disable-next-line regexp/no-dupe-characters-character-class
            /((?:^|[^$\w\xA0-\uFFFF."'\])\s]|\b(?:return|yield))\s*)/.source + // Regex pattern:
            // There are 2 regex patterns here. The RegExp set notation proposal added support for nested character
            // classes if the `v` flag is present. Unfortunately, nested CCs are both context-free and incompatible
            // with the only syntax, so we have to define 2 different regex patterns.
            /\//.source + "(?:" + /(?:\[(?:[^\]\\\r\n]|\\.)*\]|\\.|[^/\\\[\r\n])+\/[dgimyus]{0,7}/.source + "|" + // `v` flag syntax. This supports 3 levels of nested character classes.
            /(?:\[(?:[^[\]\\\r\n]|\\.|\[(?:[^[\]\\\r\n]|\\.|\[(?:[^[\]\\\r\n]|\\.)*\])*\])*\]|\\.|[^/\\\[\r\n])+\/[dgimyus]{0,7}v[dgimyus]{0,7}/.source + ")" + // lookahead
            /(?=(?:\s|\/\*(?:[^*]|\*(?!\/))*\*\/)*(?:$|[\r\n,.;:})\]]|\/\/))/.source
          ),
          lookbehind: true,
          greedy: true,
          inside: {
            "regex-source": {
              pattern: /^(\/)[\s\S]+(?=\/[a-z]*$)/,
              lookbehind: true,
              alias: "language-regex",
              inside: Prism3.languages.regex
            },
            "regex-delimiter": /^\/|\/$/,
            "regex-flags": /^[a-z]+$/
          }
        },
        // This must be declared before keyword because we use "function" inside the look-forward
        "function-variable": {
          pattern: /#?(?!\s)[_$a-zA-Z\xA0-\uFFFF](?:(?!\s)[$\w\xA0-\uFFFF])*(?=\s*[=:]\s*(?:async\s*)?(?:\bfunction\b|(?:\((?:[^()]|\([^()]*\))*\)|(?!\s)[_$a-zA-Z\xA0-\uFFFF](?:(?!\s)[$\w\xA0-\uFFFF])*)\s*=>))/,
          alias: "function"
        },
        "parameter": [
          {
            pattern: /(function(?:\s+(?!\s)[_$a-zA-Z\xA0-\uFFFF](?:(?!\s)[$\w\xA0-\uFFFF])*)?\s*\(\s*)(?!\s)(?:[^()\s]|\s+(?![\s)])|\([^()]*\))+(?=\s*\))/,
            lookbehind: true,
            inside: Prism3.languages.javascript
          },
          {
            pattern: /(^|[^$\w\xA0-\uFFFF])(?!\s)[_$a-z\xA0-\uFFFF](?:(?!\s)[$\w\xA0-\uFFFF])*(?=\s*=>)/i,
            lookbehind: true,
            inside: Prism3.languages.javascript
          },
          {
            pattern: /(\(\s*)(?!\s)(?:[^()\s]|\s+(?![\s)])|\([^()]*\))+(?=\s*\)\s*=>)/,
            lookbehind: true,
            inside: Prism3.languages.javascript
          },
          {
            pattern: /((?:\b|\s|^)(?!(?:as|async|await|break|case|catch|class|const|continue|debugger|default|delete|do|else|enum|export|extends|finally|for|from|function|get|if|implements|import|in|instanceof|interface|let|new|null|of|package|private|protected|public|return|set|static|super|switch|this|throw|try|typeof|undefined|var|void|while|with|yield)(?![$\w\xA0-\uFFFF]))(?:(?!\s)[_$a-zA-Z\xA0-\uFFFF](?:(?!\s)[$\w\xA0-\uFFFF])*\s*)\(\s*|\]\s*\(\s*)(?!\s)(?:[^()\s]|\s+(?![\s)])|\([^()]*\))+(?=\s*\)\s*\{)/,
            lookbehind: true,
            inside: Prism3.languages.javascript
          }
        ],
        "constant": /\b[A-Z](?:[A-Z_]|\dx?)*\b/
      });
      Prism3.languages.insertBefore("javascript", "string", {
        "hashbang": {
          pattern: /^#!.*/,
          greedy: true,
          alias: "comment"
        },
        "template-string": {
          pattern: /`(?:\\[\s\S]|\$\{(?:[^{}]|\{(?:[^{}]|\{[^}]*\})*\})+\}|(?!\$\{)[^\\`])*`/,
          greedy: true,
          inside: {
            "template-punctuation": {
              pattern: /^`|`$/,
              alias: "string"
            },
            "interpolation": {
              pattern: /((?:^|[^\\])(?:\\{2})*)\$\{(?:[^{}]|\{(?:[^{}]|\{[^}]*\})*\})+\}/,
              lookbehind: true,
              inside: {
                "interpolation-punctuation": {
                  pattern: /^\$\{|\}$/,
                  alias: "punctuation"
                },
                rest: Prism3.languages.javascript
              }
            },
            "string": /[\s\S]+/
          }
        },
        "string-property": {
          pattern: /((?:^|[,{])[ \t]*)(["'])(?:\\(?:\r\n|[\s\S])|(?!\2)[^\\\r\n])*\2(?=\s*:)/m,
          lookbehind: true,
          greedy: true,
          alias: "property"
        }
      });
      Prism3.languages.insertBefore("javascript", "operator", {
        "literal-property": {
          pattern: /((?:^|[,{])[ \t]*)(?!\s)[_$a-zA-Z\xA0-\uFFFF](?:(?!\s)[$\w\xA0-\uFFFF])*(?=\s*:)/m,
          lookbehind: true,
          alias: "property"
        }
      });
      if (Prism3.languages.markup) {
        Prism3.languages.markup.tag.addInlined("script", "javascript");
        Prism3.languages.markup.tag.addAttribute(
          /on(?:abort|blur|change|click|composition(?:end|start|update)|dblclick|error|focus(?:in|out)?|key(?:down|up)|load|mouse(?:down|enter|leave|move|out|over|up)|reset|resize|scroll|select|slotchange|submit|unload|wheel)/.source,
          "javascript"
        );
      }
      Prism3.languages.js = Prism3.languages.javascript;
      (function() {
        if (typeof Prism3 === "undefined" || typeof document === "undefined") {
          return;
        }
        if (!Element.prototype.matches) {
          Element.prototype.matches = Element.prototype.msMatchesSelector || Element.prototype.webkitMatchesSelector;
        }
        var LOADING_MESSAGE = "Loading\u2026";
        var FAILURE_MESSAGE = function(status, message) {
          return "\u2716 Error " + status + " while fetching file: " + message;
        };
        var FAILURE_EMPTY_MESSAGE = "\u2716 Error: File does not exist or is empty";
        var EXTENSIONS = {
          "js": "javascript",
          "py": "python",
          "rb": "ruby",
          "ps1": "powershell",
          "psm1": "powershell",
          "sh": "bash",
          "bat": "batch",
          "h": "c",
          "tex": "latex"
        };
        var STATUS_ATTR = "data-src-status";
        var STATUS_LOADING = "loading";
        var STATUS_LOADED = "loaded";
        var STATUS_FAILED = "failed";
        var SELECTOR = "pre[data-src]:not([" + STATUS_ATTR + '="' + STATUS_LOADED + '"]):not([' + STATUS_ATTR + '="' + STATUS_LOADING + '"])';
        function loadFile(src, success, error) {
          var xhr = new XMLHttpRequest();
          xhr.open("GET", src, true);
          xhr.onreadystatechange = function() {
            if (xhr.readyState == 4) {
              if (xhr.status < 400 && xhr.responseText) {
                success(xhr.responseText);
              } else {
                if (xhr.status >= 400) {
                  error(FAILURE_MESSAGE(xhr.status, xhr.statusText));
                } else {
                  error(FAILURE_EMPTY_MESSAGE);
                }
              }
            }
          };
          xhr.send(null);
        }
        function parseRange(range) {
          var m = /^\s*(\d+)\s*(?:(,)\s*(?:(\d+)\s*)?)?$/.exec(range || "");
          if (m) {
            var start = Number(m[1]);
            var comma = m[2];
            var end = m[3];
            if (!comma) {
              return [start, start];
            }
            if (!end) {
              return [start, void 0];
            }
            return [start, Number(end)];
          }
          return void 0;
        }
        Prism3.hooks.add("before-highlightall", function(env) {
          env.selector += ", " + SELECTOR;
        });
        Prism3.hooks.add("before-sanity-check", function(env) {
          var pre = (
            /** @type {HTMLPreElement} */
            env.element
          );
          if (pre.matches(SELECTOR)) {
            env.code = "";
            pre.setAttribute(STATUS_ATTR, STATUS_LOADING);
            var code = pre.appendChild(document.createElement("CODE"));
            code.textContent = LOADING_MESSAGE;
            var src = pre.getAttribute("data-src");
            var language = env.language;
            if (language === "none") {
              var extension = (/\.(\w+)$/.exec(src) || [, "none"])[1];
              language = EXTENSIONS[extension] || extension;
            }
            Prism3.util.setLanguage(code, language);
            Prism3.util.setLanguage(pre, language);
            var autoloader = Prism3.plugins.autoloader;
            if (autoloader) {
              autoloader.loadLanguages(language);
            }
            loadFile(
              src,
              function(text) {
                pre.setAttribute(STATUS_ATTR, STATUS_LOADED);
                var range = parseRange(pre.getAttribute("data-range"));
                if (range) {
                  var lines = text.split(/\r\n?|\n/g);
                  var start = range[0];
                  var end = range[1] == null ? lines.length : range[1];
                  if (start < 0) {
                    start += lines.length;
                  }
                  start = Math.max(0, Math.min(start - 1, lines.length));
                  if (end < 0) {
                    end += lines.length;
                  }
                  end = Math.max(0, Math.min(end, lines.length));
                  text = lines.slice(start, end).join("\n");
                  if (!pre.hasAttribute("data-start")) {
                    pre.setAttribute("data-start", String(start + 1));
                  }
                }
                code.textContent = text;
                Prism3.highlightElement(code);
              },
              function(error) {
                pre.setAttribute(STATUS_ATTR, STATUS_FAILED);
                code.textContent = error;
              }
            );
          }
        });
        Prism3.plugins.fileHighlight = {
          /**
           * Executes the File Highlight plugin for all matching `pre` elements under the given container.
           *
           * Note: Elements which are already loaded or currently loading will not be touched by this method.
           *
           * @param {ParentNode} [container=document]
           */
          highlight: function highlight2(container) {
            var elements = (container || document).querySelectorAll(SELECTOR);
            for (var i = 0, element; element = elements[i++]; ) {
              Prism3.highlightElement(element);
            }
          }
        };
        var logged = false;
        Prism3.fileHighlight = function() {
          if (!logged) {
            console.warn("Prism.fileHighlight is deprecated. Use `Prism.plugins.fileHighlight.highlight` instead.");
            logged = true;
          }
          Prism3.plugins.fileHighlight.highlight.apply(this, arguments);
        };
      })();
    }
  });

  // src/webkit.ts
  var webkit_exports = {};
  __export(webkit_exports, {
    bootSnippet: () => bootSnippet,
    el: () => el,
    enhanceProse: () => enhanceProse,
    escapeHtml: () => escapeHtml,
    init: () => init,
    openCmdK: () => openCmdK,
    openFeatureGuide: () => openFeatureGuide,
    poll: () => poll,
    segmentSentences: () => segmentSentences,
    stopReadAloud: () => stopReadAloud,
    toFixation: () => toFixation
  });

  // src/fixation.ts
  function toFixation(text) {
    return text.replace(/\b([a-zA-Z]+)\b/g, (word) => {
      if (word.length <= 1) return word;
      const mid = Math.ceil(word.length / 2);
      return "<b>" + word.slice(0, mid) + "</b>" + word.slice(mid);
    });
  }

  // src/size.ts
  var SIZE_MIN = 12;
  var SIZE_MAX = 24;
  var SIZE_STEP = 1;
  var SIZE_DEFAULT = 16;
  function clampSize(px) {
    return Math.max(SIZE_MIN, Math.min(SIZE_MAX, px));
  }

  // src/sentences.ts
  function speakable(text) {
    return text.replace(/[`"“”„«»]/g, "").replace(/\s+/g, " ").trim();
  }
  function segmentSentences(text, base = 0) {
    const out = [];
    if (typeof Intl !== "undefined" && typeof Intl.Segmenter === "function") {
      const seg = new Intl.Segmenter(void 0, { granularity: "sentence" });
      for (const s of seg.segment(text)) {
        const trimmed = s.segment.trim();
        if (trimmed) {
          out.push({ text: trimmed, start: base + s.index, end: base + s.index + s.segment.length });
        }
      }
      return out;
    }
    const re = /[^.!?]+[.!?]+\s*|[^.!?]+$/g;
    let m;
    while ((m = re.exec(text)) !== null) {
      const trimmed = m[0].trim();
      if (trimmed) {
        out.push({ text: trimmed, start: base + m.index, end: base + m.index + m[0].length });
      }
    }
    return out;
  }
  var MAX_PART_CHARS = 600;
  var LIVE_RAMP = [0, 250, MAX_PART_CHARS];
  function bound(ramp, i) {
    if (!ramp.length) return MAX_PART_CHARS;
    return ramp[Math.min(i, ramp.length - 1)];
  }
  function charCount(s) {
    return Array.from(s).length;
  }
  function group(pieces, ramp) {
    const parts = [];
    let cur = [];
    let size = 0;
    pieces.forEach((piece, i) => {
      const n = charCount(piece);
      if (cur.length) {
        const limit = bound(ramp, parts.length);
        if (limit === 0 || size + 1 + n > limit) {
          parts.push(cur);
          cur = [];
          size = 0;
        } else {
          size++;
        }
      }
      cur.push(i);
      size += n;
    });
    if (cur.length) parts.push(cur);
    return parts;
  }
  var CLOSERS = /[)\]}'’»]+$/u;
  var TERMINAL = ".!?:;\u2026";
  function terminate(s) {
    const core = s.replace(CLOSERS, "");
    if (!core) return s;
    const last = Array.from(core).pop() ?? "";
    return TERMINAL.includes(last) ? s : s + ".";
  }
  function joinParts(pieces) {
    return pieces.map((p) => p.trim()).filter((p) => p).map(terminate).join(" ");
  }
  function splitKeys(list) {
    return (list ?? "").split(/\s+/).filter((k) => k);
  }
  function partKeys(planned, blocks) {
    const plan = splitKeys(planned);
    if (plan.length) return plan;
    return [...new Set(blocks.flatMap(splitKeys))];
  }

  // src/read-aloud.ts
  var DEFAULT_ENDPOINT = "http://speak.this";
  var DEFAULT_VOICE = "af_heart";
  var TTS_MODEL = "mlx-community/Kokoro-82M-bf16";
  var PROBE_TIMEOUT_MS = 1500;
  var ERROR_TOAST_MS = 8e3;
  var SKIP_TAGS = /* @__PURE__ */ new Set([
    "PRE",
    "TABLE",
    "SVG",
    "SCRIPT",
    "STYLE",
    "BUTTON",
    "SELECT",
    "TEXTAREA",
    "WK-BADGE"
  ]);
  var BLOCK_TAGS = /* @__PURE__ */ new Set([
    "P",
    "LI",
    "TD",
    "TH",
    "DIV",
    "SECTION",
    "ARTICLE",
    "BLOCKQUOTE",
    "H1",
    "H2",
    "H3",
    "H4",
    "H5",
    "H6",
    "UL",
    "OL",
    "TR",
    "BR",
    "WK-TITLE",
    "WK-SUBTITLE",
    "WK-SECTION-HEADING",
    "WK-SECTION-SUBHEADING",
    "WK-CALLOUT",
    "WK-KV-ROW",
    "WK-PANEL-TITLE",
    "WK-PANEL-SUBTITLE",
    "WK-TOC-TITLE",
    "WK-PROGRESS-LABEL",
    "WK-CARD"
  ]);
  var SVG_PLAY = '<svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M8 5v14l11-7z"/></svg>';
  var SVG_PAUSE = '<svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M6 5h4v14H6zM14 5h4v14h-4z"/></svg>';
  var SVG_RESTART = '<svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M6 6h2v12H6zm3.5 6l8.5 6V6z"/></svg>';
  function collectRuns(root) {
    const runs = [];
    let current = [];
    const flush = () => {
      if (current.length) {
        runs.push(current);
        current = [];
      }
    };
    const walk = (node) => {
      if (node.nodeType === Node.TEXT_NODE) {
        current.push(node);
        return;
      }
      if (node.nodeType !== Node.ELEMENT_NODE) return;
      const tag = node.tagName;
      if (SKIP_TAGS.has(tag)) return;
      const isBlock = BLOCK_TAGS.has(tag);
      if (isBlock) flush();
      Array.from(node.childNodes).forEach(walk);
      if (isBlock) flush();
    };
    walk(root);
    flush();
    return runs;
  }
  function planSection(section) {
    const nodes = [];
    const sentences = [];
    let offset = 0;
    for (const run of collectRuns(section)) {
      let runText = "";
      for (const n of run) {
        runText += n.textContent ?? "";
        nodes.push(n);
      }
      sentences.push(...segmentSentences(runText, offset));
      offset += runText.length;
    }
    return { sentences, nodes };
  }
  function wrapSentences(nodes, sentences) {
    const byIdx = /* @__PURE__ */ new Map();
    let offset = 0;
    for (const node of nodes) {
      const text = node.textContent ?? "";
      const nodeStart = offset;
      const nodeEnd = offset + text.length;
      offset = nodeEnd;
      if (!text || !node.parentNode) continue;
      const overlaps = sentences.map((s, i) => ({ s, i })).filter(({ s }) => s.start < nodeEnd && s.end > nodeStart);
      if (!overlaps.length) continue;
      const frag = document.createDocumentFragment();
      let pos = nodeStart;
      for (const { s, i } of overlaps) {
        const from = Math.max(s.start, nodeStart);
        const to = Math.min(s.end, nodeEnd);
        if (from > pos) {
          frag.appendChild(document.createTextNode(text.slice(pos - nodeStart, from - nodeStart)));
        }
        const span = document.createElement("span");
        span.className = "wk-ra-sentence";
        span.textContent = text.slice(from - nodeStart, to - nodeStart);
        frag.appendChild(span);
        const list = byIdx.get(i);
        if (list) {
          list.push(span);
        } else {
          byIdx.set(i, [span]);
        }
        pos = to;
      }
      if (pos < nodeEnd) {
        frag.appendChild(document.createTextNode(text.slice(pos - nodeStart)));
      }
      node.parentNode.replaceChild(frag, node);
    }
    return byIdx;
  }
  function unwrapSentences(section, spans) {
    for (const list of spans.values()) {
      for (const span of list) {
        span.replaceWith(document.createTextNode(span.textContent ?? ""));
      }
    }
    section.normalize();
  }
  function planCached(section, cfg) {
    const readers = /* @__PURE__ */ new Map();
    const lists = [];
    section.querySelectorAll("[data-ra-chunk]").forEach((el2) => {
      const list = el2.dataset.raChunk ?? "";
      lists.push(list);
      for (const key of splitKeys(list)) {
        const els = readers.get(key);
        if (els) {
          els.push(el2);
        } else {
          readers.set(key, [el2]);
        }
      }
    });
    const keys = partKeys(section.dataset.raParts, lists);
    if (!keys.length) return null;
    const parts = keys.map((key) => ({
      fetch: () => fetchCachedPart(cfg, key),
      highlight: readers.get(key) ?? []
    }));
    return { parts, spans: /* @__PURE__ */ new Map(), cached: true };
  }
  function sentenceParts(cfg, sentences, spans) {
    const pieces = sentences.map((s, idx) => ({ text: speakable(s.text), idx })).filter((p) => p.text);
    return group(pieces.map((p) => p.text), LIVE_RAMP).map((members) => {
      const text = joinParts(members.map((m) => pieces[m].text));
      return {
        fetch: () => fetchSpeech(cfg, text),
        highlight: members.flatMap((m) => spans.get(pieces[m].idx) ?? [])
      };
    });
  }
  function planUncached(section, cfg) {
    const { sentences, nodes } = planSection(section);
    if (!sentences.some((s) => speakable(s.text))) return null;
    const spans = wrapSentences(nodes, sentences);
    return { parts: sentenceParts(cfg, sentences, spans), spans, cached: false };
  }
  var PARTS_AHEAD = 2;
  var session = null;
  function stopReadAloud() {
    if (session) endSession(session);
  }
  async function fetchAudio(cfg, url, init2) {
    let res;
    try {
      res = await fetch(url, init2);
    } catch {
      throw new Error("the speech service at " + (cfg.endpoint || location.origin) + " is not reachable");
    }
    if (!res.ok) throw new Error(await failureReason(res));
    let blob;
    try {
      blob = await res.blob();
    } catch {
      throw new Error("the speech service broke off the audio mid-response");
    }
    if (!blob.size) throw new Error("the speech service returned empty audio");
    return URL.createObjectURL(blob);
  }
  function fetchSpeech(cfg, text) {
    return fetchAudio(cfg, cfg.endpoint + "/v1/audio/speech", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      // wav: the engine encodes mp3 via ffmpeg, which may not be installed.
      body: JSON.stringify({ model: TTS_MODEL, input: text, voice: cfg.voice, speed: cfg.speed, response_format: "wav" })
    });
  }
  function fetchCachedPart(cfg, key) {
    return fetchAudio(cfg, `${cfg.endpoint}/audio/${encodeURIComponent(key)}`);
  }
  async function failureReason(res) {
    try {
      const body = await res.json();
      const msg = body?.error?.message;
      if (typeof msg === "string" && msg) return msg;
    } catch {
    }
    return "the speech service returned " + res.status;
  }
  function failSession(s, err) {
    const reason = err instanceof Error ? err.message : String(err);
    console.warn("wk-read-aloud:", reason);
    endSession(s);
    s.btn.classList.add("wk-ra-error");
    s.btn.title = "Read-aloud failed: " + reason + ". Click to try again.";
    s.btn.setAttribute("aria-label", "Read-aloud failed. Click to try again.");
    showErrorToast("Read-aloud failed: " + reason);
    document.dispatchEvent(new CustomEvent("wk-read-aloud-error", { detail: { reason } }));
  }
  function showErrorToast(text) {
    let host = document.querySelector("wk-toast-host");
    if (!host) {
      host = document.createElement("wk-toast-host");
      document.body.appendChild(host);
    }
    const toast = document.createElement("wk-toast");
    toast.setAttribute("variant", "err");
    toast.setAttribute("role", "alert");
    toast.textContent = text;
    host.appendChild(toast);
    setTimeout(() => toast.remove(), ERROR_TOAST_MS);
  }
  function ensureClip(s, idx) {
    let p = s.pending[idx];
    if (!p) {
      p = s.plan.parts[idx].fetch();
      s.pending[idx] = p;
    }
    return p;
  }
  function prefetch(s, idx) {
    const last = Math.min(idx + PARTS_AHEAD, s.plan.parts.length - 1);
    for (let i = idx + 1; i <= last; i++) {
      void ensureClip(s, i).catch(() => {
      });
    }
  }
  function setBtn(btn, state) {
    btn.innerHTML = state === "playing" ? SVG_PAUSE : SVG_PLAY;
    btn.classList.remove("wk-ra-error");
    btn.title = btn.classList.contains("wk-ra-float") ? "Read selection aloud" : "Read aloud";
    btn.classList.toggle("active", state !== "idle");
    btn.setAttribute("aria-pressed", String(state === "playing"));
    btn.setAttribute("aria-label", state === "playing" ? "Pause reading" : "Read section aloud");
  }
  function syncWait(s) {
    if (s.cancelled) return;
    s.btn.classList.toggle("wk-ra-wait", s.waiting && !s.userPaused);
  }
  function applyRate(s) {
    const rate = s.plan.cached ? s.cfg.speed : 1;
    s.audio.defaultPlaybackRate = rate;
    s.audio.playbackRate = rate;
  }
  function highlight(s, idx) {
    const prev = s.lit;
    const cur = s.plan.parts[idx].highlight;
    prev.forEach((el2) => {
      if (!cur.includes(el2)) el2.classList.remove("wk-ra-active");
    });
    cur.forEach((el2) => el2.classList.add("wk-ra-active"));
    s.lit = cur;
    const fresh = cur.find((el2) => !prev.includes(el2));
    if (!fresh) return;
    const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    fresh.scrollIntoView({ behavior: reduced ? "auto" : "smooth", block: "nearest" });
  }
  function endSession(s) {
    s.cancelled = true;
    s.audio.pause();
    s.audio.removeAttribute("src");
    for (const p of s.pending) {
      if (p) p.then((url) => URL.revokeObjectURL(url)).catch(() => {
      });
    }
    s.lit.forEach((el2) => el2.classList.remove("wk-ra-active"));
    s.lit = [];
    if (s.plan.spans.size) unwrapSentences(s.section, s.plan.spans);
    s.btn.classList.remove("wk-ra-wait");
    setBtn(s.btn, "idle");
    if (s.restartBtn) s.restartBtn.hidden = true;
    if (session === s) session = null;
  }
  function startClip(s, url, idx) {
    s.audio.src = url;
    applyRate(s);
    s.audio.onended = () => {
      void playFrom(s, idx + 1);
    };
    return s.audio.play();
  }
  async function playFrom(s, idx) {
    if (s.cancelled) return;
    if (idx >= s.plan.parts.length) {
      endSession(s);
      return;
    }
    highlight(s, idx);
    const clip = ensureClip(s, idx);
    prefetch(s, idx);
    s.waiting = true;
    syncWait(s);
    let url;
    try {
      url = await clip;
    } catch (err) {
      if (!s.cancelled) failSession(s, err);
      return;
    } finally {
      s.waiting = false;
      syncWait(s);
    }
    if (s.cancelled) return;
    if (s.userPaused) {
      s.queued = { url, idx };
      return;
    }
    try {
      await startClip(s, url, idx);
    } catch (err) {
      if (!s.cancelled) failSession(s, err);
    }
  }
  function newSession(section, btn, restartBtn, cfg, plan) {
    const s = {
      section,
      btn,
      restartBtn,
      cfg,
      plan,
      audio: new Audio(),
      pending: new Array(plan.parts.length).fill(null),
      queued: null,
      lit: [],
      waiting: false,
      userPaused: false,
      cancelled: false
    };
    session = s;
    setBtn(btn, "playing");
    if (restartBtn) restartBtn.hidden = false;
    void playFrom(s, 0);
  }
  function startSession(section, btn, cfg, restartBtn = null) {
    if (session) endSession(session);
    const plan = planCached(section, cfg) ?? planUncached(section, cfg);
    if (!plan) return;
    newSession(section, btn, restartBtn, cfg, plan);
  }
  function startSelectionSession(text, btn, cfg) {
    if (session) endSession(session);
    const parts = sentenceParts(cfg, segmentSentences(text), /* @__PURE__ */ new Map());
    if (!parts.length) return;
    newSession(btn, btn, null, cfg, { parts, spans: /* @__PURE__ */ new Map(), cached: false });
  }
  function onButton(section, btn, cfg, restartBtn = null) {
    if (session && session.section === section) {
      const s = session;
      if (s.userPaused) {
        s.userPaused = false;
        setBtn(btn, "playing");
        syncWait(s);
        if (s.queued) {
          const q = s.queued;
          s.queued = null;
          void startClip(s, q.url, q.idx).catch(() => endSession(s));
        } else if (!s.waiting && s.audio.src) {
          void s.audio.play().catch(() => endSession(s));
        }
      } else {
        s.userPaused = true;
        s.audio.pause();
        setBtn(btn, "paused");
        syncWait(s);
      }
      return;
    }
    startSession(section, btn, cfg, restartBtn);
  }
  var MIN_SELECTION_CHARS = 2;
  function installSelectionSpeaker(cfg) {
    let floatBtn = null;
    let selectedText = "";
    const removeFloat = () => {
      floatBtn?.remove();
      floatBtn = null;
    };
    const showFloat = () => {
      const sel = window.getSelection();
      if (!sel || sel.isCollapsed) return;
      const text = sel.toString().trim();
      if (text.length < MIN_SELECTION_CHARS) return;
      const anchor = sel.anchorNode instanceof Element ? sel.anchorNode : sel.anchorNode?.parentElement;
      if (anchor?.closest(".wk-ra-btn, wk-header")) return;
      selectedText = text;
      const rect = sel.getRangeAt(0).getBoundingClientRect();
      if (!floatBtn) {
        floatBtn = document.createElement("button");
        floatBtn.className = "wk-ra-btn wk-ra-float";
        floatBtn.type = "button";
        floatBtn.title = "Read selection aloud";
        setBtn(floatBtn, "idle");
        floatBtn.setAttribute("aria-label", "Read selection aloud");
        floatBtn.addEventListener("pointerdown", (e) => e.preventDefault());
        floatBtn.addEventListener("click", () => {
          if (session && session.btn === floatBtn) {
            onButton(floatBtn, floatBtn, cfg);
            return;
          }
          startSelectionSession(selectedText, floatBtn, cfg);
        });
        document.body.appendChild(floatBtn);
      }
      floatBtn.style.left = `${Math.round(rect.right + window.scrollX + 6)}px`;
      floatBtn.style.top = `${Math.round(rect.top + window.scrollY - 6)}px`;
    };
    document.addEventListener("pointerup", () => setTimeout(showFloat, 0));
    document.addEventListener("keyup", (e) => {
      if (e.shiftKey || e.key === "Shift") setTimeout(showFloat, 0);
    });
    document.addEventListener("keydown", (e) => {
      if (e.key !== "Escape") return;
      if (session && floatBtn && session.btn === floatBtn) endSession(session);
      window.getSelection()?.removeAllRanges();
      removeFloat();
    });
    document.addEventListener("selectionchange", () => {
      const sel = window.getSelection();
      if (!sel || sel.isCollapsed) {
        if (!(session && floatBtn && session.btn === floatBtn)) removeFloat();
      }
    });
  }
  async function probe(endpoint) {
    try {
      const ctrl = new AbortController();
      const timer = setTimeout(() => ctrl.abort(), PROBE_TIMEOUT_MS);
      await fetch(endpoint + "/", { signal: ctrl.signal });
      clearTimeout(timer);
      return true;
    } catch {
      return false;
    }
  }
  var WkReadAloud = class extends HTMLElement {
    constructor() {
      super(...arguments);
      this._initialized = false;
    }
    connectedCallback() {
      if (this._initialized) return;
      this._initialized = true;
      void this._setup();
    }
    async _setup() {
      const savedSpeed = localStorage.getItem("webkit-ra-speed");
      const attrSpeed = this.getAttribute("speed");
      const initialSpeed = parseFloat(savedSpeed ?? attrSpeed ?? "1") || 1;
      const cfg = {
        endpoint: (this.getAttribute("endpoint") ?? DEFAULT_ENDPOINT).replace(/\/+$/, ""),
        voice: this.getAttribute("voice") ?? DEFAULT_VOICE,
        speed: initialSpeed
      };
      document.addEventListener("wk-speedchange", ((e) => {
        cfg.speed = e.detail.speed;
        if (session?.cfg === cfg) applyRate(session);
      }));
      if (!await probe(cfg.endpoint)) return;
      const selector = this.getAttribute("targets");
      if (selector) {
        document.querySelectorAll(selector).forEach((section) => {
          const restartBtn = document.createElement("button");
          restartBtn.className = "wk-ra-btn";
          restartBtn.type = "button";
          restartBtn.hidden = true;
          restartBtn.title = "Restart from beginning";
          restartBtn.innerHTML = SVG_RESTART;
          restartBtn.setAttribute("aria-label", "Restart from beginning");
          restartBtn.addEventListener("click", () => startSession(section, btn, cfg, restartBtn));
          const btn = document.createElement("button");
          btn.className = "wk-ra-btn";
          btn.type = "button";
          btn.title = "Read aloud";
          setBtn(btn, "idle");
          btn.addEventListener("click", () => onButton(section, btn, cfg, restartBtn));
          section.insertBefore(restartBtn, section.firstChild);
          section.insertBefore(btn, section.firstChild);
        });
      }
      installSelectionSpeaker(cfg);
    }
  };

  // src/fuzzy.ts
  function fuzzyMatch(query, target) {
    const q = query.toLowerCase();
    const t = target.toLowerCase();
    if (q === "") return { score: 0, indices: [] };
    const indices = [];
    let score = 0;
    let ti = 0;
    let prev = -1;
    for (const ch of q) {
      let found = -1;
      for (; ti < t.length; ti++) {
        if (t[ti] === ch) {
          found = ti;
          ti++;
          break;
        }
      }
      if (found === -1) return null;
      indices.push(found);
      score += 1;
      if (found === prev + 1) score += 3;
      if (found === 0) score += 5;
      else if (!/[a-z0-9]/.test(t[found - 1])) score += 2;
      prev = found;
    }
    score -= t.length * 0.05;
    score -= indices[0] * 0.1;
    return { score, indices };
  }
  function rankSites(query, sites) {
    if (query.trim() === "") return sites.map((site) => ({ site, indices: [] }));
    const scored = [];
    sites.forEach((site, order) => {
      const byName = fuzzyMatch(query, site.name);
      if (byName) {
        scored.push({ match: { site, indices: byName.indices }, score: byName.score + 1, order });
        return;
      }
      const byHost = fuzzyMatch(query, site.host ?? site.url);
      if (byHost) {
        scored.push({ match: { site, indices: [] }, score: byHost.score, order });
      }
    });
    scored.sort((a, b) => b.score - a.score || a.order - b.order);
    return scored.map((s) => s.match);
  }

  // src/prose.ts
  var import_prismjs = __toESM(require_prism(), 1);

  // node_modules/prismjs/components/prism-bash.js
  (function(Prism3) {
    var envVars = "\\b(?:BASH|BASHOPTS|BASH_ALIASES|BASH_ARGC|BASH_ARGV|BASH_CMDS|BASH_COMPLETION_COMPAT_DIR|BASH_LINENO|BASH_REMATCH|BASH_SOURCE|BASH_VERSINFO|BASH_VERSION|COLORTERM|COLUMNS|COMP_WORDBREAKS|DBUS_SESSION_BUS_ADDRESS|DEFAULTS_PATH|DESKTOP_SESSION|DIRSTACK|DISPLAY|EUID|GDMSESSION|GDM_LANG|GNOME_KEYRING_CONTROL|GNOME_KEYRING_PID|GPG_AGENT_INFO|GROUPS|HISTCONTROL|HISTFILE|HISTFILESIZE|HISTSIZE|HOME|HOSTNAME|HOSTTYPE|IFS|INSTANCE|JOB|LANG|LANGUAGE|LC_ADDRESS|LC_ALL|LC_IDENTIFICATION|LC_MEASUREMENT|LC_MONETARY|LC_NAME|LC_NUMERIC|LC_PAPER|LC_TELEPHONE|LC_TIME|LESSCLOSE|LESSOPEN|LINES|LOGNAME|LS_COLORS|MACHTYPE|MAILCHECK|MANDATORY_PATH|NO_AT_BRIDGE|OLDPWD|OPTERR|OPTIND|ORBIT_SOCKETDIR|OSTYPE|PAPERSIZE|PATH|PIPESTATUS|PPID|PS1|PS2|PS3|PS4|PWD|RANDOM|REPLY|SECONDS|SELINUX_INIT|SESSION|SESSIONTYPE|SESSION_MANAGER|SHELL|SHELLOPTS|SHLVL|SSH_AUTH_SOCK|TERM|UID|UPSTART_EVENTS|UPSTART_INSTANCE|UPSTART_JOB|UPSTART_SESSION|USER|WINDOWID|XAUTHORITY|XDG_CONFIG_DIRS|XDG_CURRENT_DESKTOP|XDG_DATA_DIRS|XDG_GREETER_DATA_DIR|XDG_MENU_PREFIX|XDG_RUNTIME_DIR|XDG_SEAT|XDG_SEAT_PATH|XDG_SESSION_DESKTOP|XDG_SESSION_ID|XDG_SESSION_PATH|XDG_SESSION_TYPE|XDG_VTNR|XMODIFIERS)\\b";
    var commandAfterHeredoc = {
      pattern: /(^(["']?)\w+\2)[ \t]+\S.*/,
      lookbehind: true,
      alias: "punctuation",
      // this looks reasonably well in all themes
      inside: null
      // see below
    };
    var insideString = {
      "bash": commandAfterHeredoc,
      "environment": {
        pattern: RegExp("\\$" + envVars),
        alias: "constant"
      },
      "variable": [
        // [0]: Arithmetic Environment
        {
          pattern: /\$?\(\([\s\S]+?\)\)/,
          greedy: true,
          inside: {
            // If there is a $ sign at the beginning highlight $(( and )) as variable
            "variable": [
              {
                pattern: /(^\$\(\([\s\S]+)\)\)/,
                lookbehind: true
              },
              /^\$\(\(/
            ],
            "number": /\b0x[\dA-Fa-f]+\b|(?:\b\d+(?:\.\d*)?|\B\.\d+)(?:[Ee]-?\d+)?/,
            // Operators according to https://www.gnu.org/software/bash/manual/bashref.html#Shell-Arithmetic
            "operator": /--|\+\+|\*\*=?|<<=?|>>=?|&&|\|\||[=!+\-*/%<>^&|]=?|[?~:]/,
            // If there is no $ sign at the beginning highlight (( and )) as punctuation
            "punctuation": /\(\(?|\)\)?|,|;/
          }
        },
        // [1]: Command Substitution
        {
          pattern: /\$\((?:\([^)]+\)|[^()])+\)|`[^`]+`/,
          greedy: true,
          inside: {
            "variable": /^\$\(|^`|\)$|`$/
          }
        },
        // [2]: Brace expansion
        {
          pattern: /\$\{[^}]+\}/,
          greedy: true,
          inside: {
            "operator": /:[-=?+]?|[!\/]|##?|%%?|\^\^?|,,?/,
            "punctuation": /[\[\]]/,
            "environment": {
              pattern: RegExp("(\\{)" + envVars),
              lookbehind: true,
              alias: "constant"
            }
          }
        },
        /\$(?:\w+|[#?*!@$])/
      ],
      // Escape sequences from echo and printf's manuals, and escaped quotes.
      "entity": /\\(?:[abceEfnrtv\\"]|O?[0-7]{1,3}|U[0-9a-fA-F]{8}|u[0-9a-fA-F]{4}|x[0-9a-fA-F]{1,2})/
    };
    Prism3.languages.bash = {
      "shebang": {
        pattern: /^#!\s*\/.*/,
        alias: "important"
      },
      "comment": {
        pattern: /(^|[^"{\\$])#.*/,
        lookbehind: true
      },
      "function-name": [
        // a) function foo {
        // b) foo() {
        // c) function foo() {
        // but not “foo {”
        {
          // a) and c)
          pattern: /(\bfunction\s+)[\w-]+(?=(?:\s*\(?:\s*\))?\s*\{)/,
          lookbehind: true,
          alias: "function"
        },
        {
          // b)
          pattern: /\b[\w-]+(?=\s*\(\s*\)\s*\{)/,
          alias: "function"
        }
      ],
      // Highlight variable names as variables in for and select beginnings.
      "for-or-select": {
        pattern: /(\b(?:for|select)\s+)\w+(?=\s+in\s)/,
        alias: "variable",
        lookbehind: true
      },
      // Highlight variable names as variables in the left-hand part
      // of assignments (“=” and “+=”).
      "assign-left": {
        pattern: /(^|[\s;|&]|[<>]\()\w+(?:\.\w+)*(?=\+?=)/,
        inside: {
          "environment": {
            pattern: RegExp("(^|[\\s;|&]|[<>]\\()" + envVars),
            lookbehind: true,
            alias: "constant"
          }
        },
        alias: "variable",
        lookbehind: true
      },
      // Highlight parameter names as variables
      "parameter": {
        pattern: /(^|\s)-{1,2}(?:\w+:[+-]?)?\w+(?:\.\w+)*(?=[=\s]|$)/,
        alias: "variable",
        lookbehind: true
      },
      "string": [
        // Support for Here-documents https://en.wikipedia.org/wiki/Here_document
        {
          pattern: /((?:^|[^<])<<-?\s*)(\w+)\s[\s\S]*?(?:\r?\n|\r)\2/,
          lookbehind: true,
          greedy: true,
          inside: insideString
        },
        // Here-document with quotes around the tag
        // → No expansion (so no “inside”).
        {
          pattern: /((?:^|[^<])<<-?\s*)(["'])(\w+)\2\s[\s\S]*?(?:\r?\n|\r)\3/,
          lookbehind: true,
          greedy: true,
          inside: {
            "bash": commandAfterHeredoc
          }
        },
        // “Normal” string
        {
          // https://www.gnu.org/software/bash/manual/html_node/Double-Quotes.html
          pattern: /(^|[^\\](?:\\\\)*)"(?:\\[\s\S]|\$\([^)]+\)|\$(?!\()|`[^`]+`|[^"\\`$])*"/,
          lookbehind: true,
          greedy: true,
          inside: insideString
        },
        {
          // https://www.gnu.org/software/bash/manual/html_node/Single-Quotes.html
          pattern: /(^|[^$\\])'[^']*'/,
          lookbehind: true,
          greedy: true
        },
        {
          // https://www.gnu.org/software/bash/manual/html_node/ANSI_002dC-Quoting.html
          pattern: /\$'(?:[^'\\]|\\[\s\S])*'/,
          greedy: true,
          inside: {
            "entity": insideString.entity
          }
        }
      ],
      "environment": {
        pattern: RegExp("\\$?" + envVars),
        alias: "constant"
      },
      "variable": insideString.variable,
      "function": {
        pattern: /(^|[\s;|&]|[<>]\()(?:add|apropos|apt|apt-cache|apt-get|aptitude|aspell|automysqlbackup|awk|basename|bash|bc|bconsole|bg|bzip2|cal|cargo|cat|cfdisk|chgrp|chkconfig|chmod|chown|chroot|cksum|clear|cmp|column|comm|composer|cp|cron|crontab|csplit|curl|cut|date|dc|dd|ddrescue|debootstrap|df|diff|diff3|dig|dir|dircolors|dirname|dirs|dmesg|docker|docker-compose|du|egrep|eject|env|ethtool|expand|expect|expr|fdformat|fdisk|fg|fgrep|file|find|fmt|fold|format|free|fsck|ftp|fuser|gawk|git|gparted|grep|groupadd|groupdel|groupmod|groups|grub-mkconfig|gzip|halt|head|hg|history|host|hostname|htop|iconv|id|ifconfig|ifdown|ifup|import|install|ip|java|jobs|join|kill|killall|less|link|ln|locate|logname|logrotate|look|lpc|lpr|lprint|lprintd|lprintq|lprm|ls|lsof|lynx|make|man|mc|mdadm|mkconfig|mkdir|mke2fs|mkfifo|mkfs|mkisofs|mknod|mkswap|mmv|more|most|mount|mtools|mtr|mutt|mv|nano|nc|netstat|nice|nl|node|nohup|notify-send|npm|nslookup|op|open|parted|passwd|paste|pathchk|ping|pkill|pnpm|podman|podman-compose|popd|pr|printcap|printenv|ps|pushd|pv|quota|quotacheck|quotactl|ram|rar|rcp|reboot|remsync|rename|renice|rev|rm|rmdir|rpm|rsync|scp|screen|sdiff|sed|sendmail|seq|service|sftp|sh|shellcheck|shuf|shutdown|sleep|slocate|sort|split|ssh|stat|strace|su|sudo|sum|suspend|swapon|sync|sysctl|tac|tail|tar|tee|time|timeout|top|touch|tr|traceroute|tsort|tty|umount|uname|unexpand|uniq|units|unrar|unshar|unzip|update-grub|uptime|useradd|userdel|usermod|users|uudecode|uuencode|v|vcpkg|vdir|vi|vim|virsh|vmstat|wait|watch|wc|wget|whereis|which|who|whoami|write|xargs|xdg-open|yarn|yes|zenity|zip|zsh|zypper)(?=$|[)\s;|&])/,
        lookbehind: true
      },
      "keyword": {
        pattern: /(^|[\s;|&]|[<>]\()(?:case|do|done|elif|else|esac|fi|for|function|if|in|select|then|until|while)(?=$|[)\s;|&])/,
        lookbehind: true
      },
      // https://www.gnu.org/software/bash/manual/html_node/Shell-Builtin-Commands.html
      "builtin": {
        pattern: /(^|[\s;|&]|[<>]\()(?:\.|:|alias|bind|break|builtin|caller|cd|command|continue|declare|echo|enable|eval|exec|exit|export|getopts|hash|help|let|local|logout|mapfile|printf|pwd|read|readarray|readonly|return|set|shift|shopt|source|test|times|trap|type|typeset|ulimit|umask|unalias|unset)(?=$|[)\s;|&])/,
        lookbehind: true,
        // Alias added to make those easier to distinguish from strings.
        alias: "class-name"
      },
      "boolean": {
        pattern: /(^|[\s;|&]|[<>]\()(?:false|true)(?=$|[)\s;|&])/,
        lookbehind: true
      },
      "file-descriptor": {
        pattern: /\B&\d\b/,
        alias: "important"
      },
      "operator": {
        // Lots of redirections here, but not just that.
        pattern: /\d?<>|>\||\+=|=[=~]?|!=?|<<[<-]?|[&\d]?>>|\d[<>]&?|[<>][&=]?|&[>&]?|\|[&|]?/,
        inside: {
          "file-descriptor": {
            pattern: /^\d/,
            alias: "important"
          }
        }
      },
      "punctuation": /\$?\(\(?|\)\)?|\.\.|[{}[\];\\]/,
      "number": {
        pattern: /(^|\s)(?:[1-9]\d*|0)(?:[.,]\d+)?\b/,
        lookbehind: true
      }
    };
    commandAfterHeredoc.inside = Prism3.languages.bash;
    var toBeCopied = [
      "comment",
      "function-name",
      "for-or-select",
      "assign-left",
      "parameter",
      "string",
      "environment",
      "function",
      "keyword",
      "builtin",
      "boolean",
      "file-descriptor",
      "operator",
      "punctuation",
      "number"
    ];
    var inside = insideString.variable[1].inside;
    for (var i = 0; i < toBeCopied.length; i++) {
      inside[toBeCopied[i]] = Prism3.languages.bash[toBeCopied[i]];
    }
    Prism3.languages.sh = Prism3.languages.bash;
    Prism3.languages.shell = Prism3.languages.bash;
  })(Prism);

  // node_modules/prismjs/components/prism-json.js
  Prism.languages.json = {
    "property": {
      pattern: /(^|[^\\])"(?:\\.|[^\\"\r\n])*"(?=\s*:)/,
      lookbehind: true,
      greedy: true
    },
    "string": {
      pattern: /(^|[^\\])"(?:\\.|[^\\"\r\n])*"(?!\s*:)/,
      lookbehind: true,
      greedy: true
    },
    "comment": {
      pattern: /\/\/.*|\/\*[\s\S]*?(?:\*\/|$)/,
      greedy: true
    },
    "number": /-?\b\d+(?:\.\d+)?(?:e[+-]?\d+)?\b/i,
    "punctuation": /[{}[\],]/,
    "operator": /:/,
    "boolean": /\b(?:false|true)\b/,
    "null": {
      pattern: /\bnull\b/,
      alias: "keyword"
    }
  };
  Prism.languages.webmanifest = Prism.languages.json;

  // node_modules/prismjs/components/prism-go.js
  Prism.languages.go = Prism.languages.extend("clike", {
    "string": {
      pattern: /(^|[^\\])"(?:\\.|[^"\\\r\n])*"|`[^`]*`/,
      lookbehind: true,
      greedy: true
    },
    "keyword": /\b(?:break|case|chan|const|continue|default|defer|else|fallthrough|for|func|go(?:to)?|if|import|interface|map|package|range|return|select|struct|switch|type|var)\b/,
    "boolean": /\b(?:_|false|iota|nil|true)\b/,
    "number": [
      // binary and octal integers
      /\b0(?:b[01_]+|o[0-7_]+)i?\b/i,
      // hexadecimal integers and floats
      /\b0x(?:[a-f\d_]+(?:\.[a-f\d_]*)?|\.[a-f\d_]+)(?:p[+-]?\d+(?:_\d+)*)?i?(?!\w)/i,
      // decimal integers and floats
      /(?:\b\d[\d_]*(?:\.[\d_]*)?|\B\.\d[\d_]*)(?:e[+-]?[\d_]+)?i?(?!\w)/i
    ],
    "operator": /[*\/%^!=]=?|\+[=+]?|-[=-]?|\|[=|]?|&(?:=|&|\^=?)?|>(?:>=?|=)?|<(?:<=?|=|-)?|:=|\.\.\./,
    "builtin": /\b(?:append|bool|byte|cap|close|complex|complex(?:64|128)|copy|delete|error|float(?:32|64)|u?int(?:8|16|32|64)?|imag|len|make|new|panic|print(?:ln)?|real|recover|rune|string|uintptr)\b/
  });
  Prism.languages.insertBefore("go", "string", {
    "char": {
      pattern: /'(?:\\.|[^'\\\r\n]){0,10}'/,
      greedy: true
    }
  });
  delete Prism.languages.go["class-name"];

  // node_modules/prismjs/components/prism-python.js
  Prism.languages.python = {
    "comment": {
      pattern: /(^|[^\\])#.*/,
      lookbehind: true,
      greedy: true
    },
    "string-interpolation": {
      pattern: /(?:f|fr|rf)(?:("""|''')[\s\S]*?\1|("|')(?:\\.|(?!\2)[^\\\r\n])*\2)/i,
      greedy: true,
      inside: {
        "interpolation": {
          // "{" <expression> <optional "!s", "!r", or "!a"> <optional ":" format specifier> "}"
          pattern: /((?:^|[^{])(?:\{\{)*)\{(?!\{)(?:[^{}]|\{(?!\{)(?:[^{}]|\{(?!\{)(?:[^{}])+\})+\})+\}/,
          lookbehind: true,
          inside: {
            "format-spec": {
              pattern: /(:)[^:(){}]+(?=\}$)/,
              lookbehind: true
            },
            "conversion-option": {
              pattern: /![sra](?=[:}]$)/,
              alias: "punctuation"
            },
            rest: null
          }
        },
        "string": /[\s\S]+/
      }
    },
    "triple-quoted-string": {
      pattern: /(?:[rub]|br|rb)?("""|''')[\s\S]*?\1/i,
      greedy: true,
      alias: "string"
    },
    "string": {
      pattern: /(?:[rub]|br|rb)?("|')(?:\\.|(?!\1)[^\\\r\n])*\1/i,
      greedy: true
    },
    "function": {
      pattern: /((?:^|\s)def[ \t]+)[a-zA-Z_]\w*(?=\s*\()/g,
      lookbehind: true
    },
    "class-name": {
      pattern: /(\bclass\s+)\w+/i,
      lookbehind: true
    },
    "decorator": {
      pattern: /(^[\t ]*)@\w+(?:\.\w+)*/m,
      lookbehind: true,
      alias: ["annotation", "punctuation"],
      inside: {
        "punctuation": /\./
      }
    },
    "keyword": /\b(?:_(?=\s*:)|and|as|assert|async|await|break|case|class|continue|def|del|elif|else|except|exec|finally|for|from|global|if|import|in|is|lambda|match|nonlocal|not|or|pass|print|raise|return|try|while|with|yield)\b/,
    "builtin": /\b(?:__import__|abs|all|any|apply|ascii|basestring|bin|bool|buffer|bytearray|bytes|callable|chr|classmethod|cmp|coerce|compile|complex|delattr|dict|dir|divmod|enumerate|eval|execfile|file|filter|float|format|frozenset|getattr|globals|hasattr|hash|help|hex|id|input|int|intern|isinstance|issubclass|iter|len|list|locals|long|map|max|memoryview|min|next|object|oct|open|ord|pow|property|range|raw_input|reduce|reload|repr|reversed|round|set|setattr|slice|sorted|staticmethod|str|sum|super|tuple|type|unichr|unicode|vars|xrange|zip)\b/,
    "boolean": /\b(?:False|None|True)\b/,
    "number": /\b0(?:b(?:_?[01])+|o(?:_?[0-7])+|x(?:_?[a-f0-9])+)\b|(?:\b\d+(?:_\d+)*(?:\.(?:\d+(?:_\d+)*)?)?|\B\.\d+(?:_\d+)*)(?:e[+-]?\d+(?:_\d+)*)?j?(?!\w)/i,
    "operator": /[-+%=]=?|!=|:=|\*\*?=?|\/\/?=?|<[<=>]?|>[=>]?|[&|^~]/,
    "punctuation": /[{}[\];(),.:]/
  };
  Prism.languages.python["string-interpolation"].inside["interpolation"].inside.rest = Prism.languages.python;
  Prism.languages.py = Prism.languages.python;

  // node_modules/prismjs/components/prism-typescript.js
  (function(Prism3) {
    Prism3.languages.typescript = Prism3.languages.extend("javascript", {
      "class-name": {
        pattern: /(\b(?:class|extends|implements|instanceof|interface|new|type)\s+)(?!keyof\b)(?!\s)[_$a-zA-Z\xA0-\uFFFF](?:(?!\s)[$\w\xA0-\uFFFF])*(?:\s*<(?:[^<>]|<(?:[^<>]|<[^<>]*>)*>)*>)?/,
        lookbehind: true,
        greedy: true,
        inside: null
        // see below
      },
      "builtin": /\b(?:Array|Function|Promise|any|boolean|console|never|number|string|symbol|unknown)\b/
    });
    Prism3.languages.typescript.keyword.push(
      /\b(?:abstract|declare|is|keyof|readonly|require)\b/,
      // keywords that have to be followed by an identifier
      /\b(?:asserts|infer|interface|module|namespace|type)\b(?=\s*(?:[{_$a-zA-Z\xA0-\uFFFF]|$))/,
      // This is for `import type *, {}`
      /\btype\b(?=\s*(?:[\{*]|$))/
    );
    delete Prism3.languages.typescript["parameter"];
    delete Prism3.languages.typescript["literal-property"];
    var typeInside = Prism3.languages.extend("typescript", {});
    delete typeInside["class-name"];
    Prism3.languages.typescript["class-name"].inside = typeInside;
    Prism3.languages.insertBefore("typescript", "function", {
      "decorator": {
        pattern: /@[$\w\xA0-\uFFFF]+/,
        inside: {
          "at": {
            pattern: /^@/,
            alias: "operator"
          },
          "function": /^[\s\S]+/
        }
      },
      "generic-function": {
        // e.g. foo<T extends "bar" | "baz">( ...
        pattern: /#?(?!\s)[_$a-zA-Z\xA0-\uFFFF](?:(?!\s)[$\w\xA0-\uFFFF])*\s*<(?:[^<>]|<(?:[^<>]|<[^<>]*>)*>)*>(?=\s*\()/,
        greedy: true,
        inside: {
          "function": /^#?(?!\s)[_$a-zA-Z\xA0-\uFFFF](?:(?!\s)[$\w\xA0-\uFFFF])*/,
          "generic": {
            pattern: /<[\s\S]+/,
            // everything after the first <
            alias: "class-name",
            inside: typeInside
          }
        }
      }
    });
    Prism3.languages.ts = Prism3.languages.typescript;
  })(Prism);

  // node_modules/prismjs/components/prism-yaml.js
  (function(Prism3) {
    var anchorOrAlias = /[*&][^\s[\]{},]+/;
    var tag = /!(?:<[\w\-%#;/?:@&=+$,.!~*'()[\]]+>|(?:[a-zA-Z\d-]*!)?[\w\-%#;/?:@&=+$.~*'()]+)?/;
    var properties = "(?:" + tag.source + "(?:[ 	]+" + anchorOrAlias.source + ")?|" + anchorOrAlias.source + "(?:[ 	]+" + tag.source + ")?)";
    var plainKey = /(?:[^\s\x00-\x08\x0e-\x1f!"#%&'*,\-:>?@[\]`{|}\x7f-\x84\x86-\x9f\ud800-\udfff\ufffe\uffff]|[?:-]<PLAIN>)(?:[ \t]*(?:(?![#:])<PLAIN>|:<PLAIN>))*/.source.replace(/<PLAIN>/g, function() {
      return /[^\s\x00-\x08\x0e-\x1f,[\]{}\x7f-\x84\x86-\x9f\ud800-\udfff\ufffe\uffff]/.source;
    });
    var string = /"(?:[^"\\\r\n]|\\.)*"|'(?:[^'\\\r\n]|\\.)*'/.source;
    function createValuePattern(value, flags) {
      flags = (flags || "").replace(/m/g, "") + "m";
      var pattern = /([:\-,[{]\s*(?:\s<<prop>>[ \t]+)?)(?:<<value>>)(?=[ \t]*(?:$|,|\]|\}|(?:[\r\n]\s*)?#))/.source.replace(/<<prop>>/g, function() {
        return properties;
      }).replace(/<<value>>/g, function() {
        return value;
      });
      return RegExp(pattern, flags);
    }
    Prism3.languages.yaml = {
      "scalar": {
        pattern: RegExp(/([\-:]\s*(?:\s<<prop>>[ \t]+)?[|>])[ \t]*(?:((?:\r?\n|\r)[ \t]+)\S[^\r\n]*(?:\2[^\r\n]+)*)/.source.replace(/<<prop>>/g, function() {
          return properties;
        })),
        lookbehind: true,
        alias: "string"
      },
      "comment": /#.*/,
      "key": {
        pattern: RegExp(/((?:^|[:\-,[{\r\n?])[ \t]*(?:<<prop>>[ \t]+)?)<<key>>(?=\s*:\s)/.source.replace(/<<prop>>/g, function() {
          return properties;
        }).replace(/<<key>>/g, function() {
          return "(?:" + plainKey + "|" + string + ")";
        })),
        lookbehind: true,
        greedy: true,
        alias: "atrule"
      },
      "directive": {
        pattern: /(^[ \t]*)%.+/m,
        lookbehind: true,
        alias: "important"
      },
      "datetime": {
        pattern: createValuePattern(/\d{4}-\d\d?-\d\d?(?:[tT]|[ \t]+)\d\d?:\d{2}:\d{2}(?:\.\d*)?(?:[ \t]*(?:Z|[-+]\d\d?(?::\d{2})?))?|\d{4}-\d{2}-\d{2}|\d\d?:\d{2}(?::\d{2}(?:\.\d*)?)?/.source),
        lookbehind: true,
        alias: "number"
      },
      "boolean": {
        pattern: createValuePattern(/false|true/.source, "i"),
        lookbehind: true,
        alias: "important"
      },
      "null": {
        pattern: createValuePattern(/null|~/.source, "i"),
        lookbehind: true,
        alias: "important"
      },
      "string": {
        pattern: createValuePattern(string),
        lookbehind: true,
        greedy: true
      },
      "number": {
        pattern: createValuePattern(/[+-]?(?:0x[\da-f]+|0o[0-7]+|(?:\d+(?:\.\d*)?|\.\d+)(?:e[+-]?\d+)?|\.inf|\.nan)/.source, "i"),
        lookbehind: true
      },
      "tag": tag,
      "important": anchorOrAlias,
      "punctuation": /---|[:[\]{}\-,|>?]|\.\.\./
    };
    Prism3.languages.yml = Prism3.languages.yaml;
  })(Prism);

  // node_modules/prismjs/components/prism-sql.js
  Prism.languages.sql = {
    "comment": {
      pattern: /(^|[^\\])(?:\/\*[\s\S]*?\*\/|(?:--|\/\/|#).*)/,
      lookbehind: true
    },
    "variable": [
      {
        pattern: /@(["'`])(?:\\[\s\S]|(?!\1)[^\\])+\1/,
        greedy: true
      },
      /@[\w.$]+/
    ],
    "string": {
      pattern: /(^|[^@\\])("|')(?:\\[\s\S]|(?!\2)[^\\]|\2\2)*\2/,
      greedy: true,
      lookbehind: true
    },
    "identifier": {
      pattern: /(^|[^@\\])`(?:\\[\s\S]|[^`\\]|``)*`/,
      greedy: true,
      lookbehind: true,
      inside: {
        "punctuation": /^`|`$/
      }
    },
    "function": /\b(?:AVG|COUNT|FIRST|FORMAT|LAST|LCASE|LEN|MAX|MID|MIN|MOD|NOW|ROUND|SUM|UCASE)(?=\s*\()/i,
    // Should we highlight user defined functions too?
    "keyword": /\b(?:ACTION|ADD|AFTER|ALGORITHM|ALL|ALTER|ANALYZE|ANY|APPLY|AS|ASC|AUTHORIZATION|AUTO_INCREMENT|BACKUP|BDB|BEGIN|BERKELEYDB|BIGINT|BINARY|BIT|BLOB|BOOL|BOOLEAN|BREAK|BROWSE|BTREE|BULK|BY|CALL|CASCADED?|CASE|CHAIN|CHAR(?:ACTER|SET)?|CHECK(?:POINT)?|CLOSE|CLUSTERED|COALESCE|COLLATE|COLUMNS?|COMMENT|COMMIT(?:TED)?|COMPUTE|CONNECT|CONSISTENT|CONSTRAINT|CONTAINS(?:TABLE)?|CONTINUE|CONVERT|CREATE|CROSS|CURRENT(?:_DATE|_TIME|_TIMESTAMP|_USER)?|CURSOR|CYCLE|DATA(?:BASES?)?|DATE(?:TIME)?|DAY|DBCC|DEALLOCATE|DEC|DECIMAL|DECLARE|DEFAULT|DEFINER|DELAYED|DELETE|DELIMITERS?|DENY|DESC|DESCRIBE|DETERMINISTIC|DISABLE|DISCARD|DISK|DISTINCT|DISTINCTROW|DISTRIBUTED|DO|DOUBLE|DROP|DUMMY|DUMP(?:FILE)?|DUPLICATE|ELSE(?:IF)?|ENABLE|ENCLOSED|END|ENGINE|ENUM|ERRLVL|ERRORS|ESCAPED?|EXCEPT|EXEC(?:UTE)?|EXISTS|EXIT|EXPLAIN|EXTENDED|FETCH|FIELDS|FILE|FILLFACTOR|FIRST|FIXED|FLOAT|FOLLOWING|FOR(?: EACH ROW)?|FORCE|FOREIGN|FREETEXT(?:TABLE)?|FROM|FULL|FUNCTION|GEOMETRY(?:COLLECTION)?|GLOBAL|GOTO|GRANT|GROUP|HANDLER|HASH|HAVING|HOLDLOCK|HOUR|IDENTITY(?:COL|_INSERT)?|IF|IGNORE|IMPORT|INDEX|INFILE|INNER|INNODB|INOUT|INSERT|INT|INTEGER|INTERSECT|INTERVAL|INTO|INVOKER|ISOLATION|ITERATE|JOIN|KEYS?|KILL|LANGUAGE|LAST|LEAVE|LEFT|LEVEL|LIMIT|LINENO|LINES|LINESTRING|LOAD|LOCAL|LOCK|LONG(?:BLOB|TEXT)|LOOP|MATCH(?:ED)?|MEDIUM(?:BLOB|INT|TEXT)|MERGE|MIDDLEINT|MINUTE|MODE|MODIFIES|MODIFY|MONTH|MULTI(?:LINESTRING|POINT|POLYGON)|NATIONAL|NATURAL|NCHAR|NEXT|NO|NONCLUSTERED|NULLIF|NUMERIC|OFF?|OFFSETS?|ON|OPEN(?:DATASOURCE|QUERY|ROWSET)?|OPTIMIZE|OPTION(?:ALLY)?|ORDER|OUT(?:ER|FILE)?|OVER|PARTIAL|PARTITION|PERCENT|PIVOT|PLAN|POINT|POLYGON|PRECEDING|PRECISION|PREPARE|PREV|PRIMARY|PRINT|PRIVILEGES|PROC(?:EDURE)?|PUBLIC|PURGE|QUICK|RAISERROR|READS?|REAL|RECONFIGURE|REFERENCES|RELEASE|RENAME|REPEAT(?:ABLE)?|REPLACE|REPLICATION|REQUIRE|RESIGNAL|RESTORE|RESTRICT|RETURN(?:ING|S)?|REVOKE|RIGHT|ROLLBACK|ROUTINE|ROW(?:COUNT|GUIDCOL|S)?|RTREE|RULE|SAVE(?:POINT)?|SCHEMA|SECOND|SELECT|SERIAL(?:IZABLE)?|SESSION(?:_USER)?|SET(?:USER)?|SHARE|SHOW|SHUTDOWN|SIMPLE|SMALLINT|SNAPSHOT|SOME|SONAME|SQL|START(?:ING)?|STATISTICS|STATUS|STRIPED|SYSTEM_USER|TABLES?|TABLESPACE|TEMP(?:ORARY|TABLE)?|TERMINATED|TEXT(?:SIZE)?|THEN|TIME(?:STAMP)?|TINY(?:BLOB|INT|TEXT)|TOP?|TRAN(?:SACTIONS?)?|TRIGGER|TRUNCATE|TSEQUAL|TYPES?|UNBOUNDED|UNCOMMITTED|UNDEFINED|UNION|UNIQUE|UNLOCK|UNPIVOT|UNSIGNED|UPDATE(?:TEXT)?|USAGE|USE|USER|USING|VALUES?|VAR(?:BINARY|CHAR|CHARACTER|YING)|VIEW|WAITFOR|WARNINGS|WHEN|WHERE|WHILE|WITH(?: ROLLUP|IN)?|WORK|WRITE(?:TEXT)?|YEAR)\b/i,
    "boolean": /\b(?:FALSE|NULL|TRUE)\b/i,
    "number": /\b0x[\da-f]+\b|\b\d+(?:\.\d*)?|\B\.\d+\b/i,
    "operator": /[-+*\/=%^~]|&&?|\|\|?|!=?|<(?:=>?|<|>)?|>[>=]?|\b(?:AND|BETWEEN|DIV|ILIKE|IN|IS|LIKE|NOT|OR|REGEXP|RLIKE|SOUNDS LIKE|XOR)\b/i,
    "punctuation": /[;[\]()`,.]/
  };

  // src/prose.ts
  import_prismjs.default.manual = true;
  var LANG_RE = /\blanguage-(\w+)/;
  function langOf(code) {
    const m = code.className.match(LANG_RE);
    return m ? m[1] : "";
  }
  function enhanceBlock(pre, code) {
    if (pre.dataset.wkProse === "1") return;
    pre.dataset.wkProse = "1";
    import_prismjs.default.highlightElement(code);
    const lang = langOf(code);
    const controls = document.createElement("div");
    controls.className = "wk-code-controls";
    const label = document.createElement("span");
    label.className = "wk-code-lang";
    label.textContent = lang || "text";
    controls.appendChild(label);
    const copy = document.createElement("button");
    copy.className = "wk-code-copy";
    copy.type = "button";
    copy.textContent = "Copy";
    copy.setAttribute("aria-label", "Copy code to clipboard");
    copy.addEventListener("click", () => {
      const text = code.textContent ?? "";
      const done = () => {
        copy.textContent = "Copied";
        copy.classList.add("wk-copied");
        setTimeout(() => {
          copy.textContent = "Copy";
          copy.classList.remove("wk-copied");
        }, 1500);
      };
      if (navigator.clipboard?.writeText) {
        navigator.clipboard.writeText(text).then(done).catch(() => {
        });
      } else {
        const ta = document.createElement("textarea");
        ta.value = text;
        ta.style.position = "fixed";
        ta.style.opacity = "0";
        document.body.appendChild(ta);
        ta.select();
        try {
          document.execCommand("copy");
          done();
        } catch {
        }
        ta.remove();
      }
    });
    controls.appendChild(copy);
    pre.classList.add("wk-has-controls");
    pre.appendChild(controls);
  }
  function enhanceProse(root = document) {
    const blocks = root.querySelectorAll(
      '.wk-prose pre > code[class*="language-"], pre.wk-code-block > code[class*="language-"]'
    );
    blocks.forEach((code) => {
      const pre = code.parentElement;
      if (pre instanceof HTMLElement) enhanceBlock(pre, code);
    });
  }
  function initProse() {
    const run = () => {
      if (!document.querySelector(".wk-prose, pre.wk-code-block")) return;
      enhanceProse(document);
    };
    if (document.readyState === "loading") {
      document.addEventListener("DOMContentLoaded", run, { once: true });
    } else {
      run();
    }
  }

  // src/render.ts
  var HTML_ENTITIES = {
    "&": "&amp;",
    "<": "&lt;",
    ">": "&gt;",
    '"': "&quot;",
    "'": "&#39;"
  };
  function escapeHtml(s) {
    return String(s).replace(/[&<>"']/g, (c) => HTML_ENTITIES[c]);
  }
  function el(tag, attrs = {}, children = []) {
    const node = document.createElement(tag);
    for (const [k, v] of Object.entries(attrs)) {
      if (k === "class") node.className = String(v);
      else if (k === "html") node.innerHTML = String(v);
      else if (k.startsWith("on") && typeof v === "function") {
        node.addEventListener(k.slice(2), v);
      } else if (v !== null && v !== void 0) {
        node.setAttribute(k, String(v));
      }
    }
    for (const c of [].concat(children)) {
      if (c == null) continue;
      node.appendChild(typeof c === "string" ? document.createTextNode(c) : c);
    }
    return node;
  }
  function poll(url, onData, ms, onError) {
    let stopped = false;
    async function tick() {
      try {
        const res = await fetch(url, { headers: { accept: "application/json" } });
        if (!res.ok) throw new Error("request failed (" + res.status + ")");
        const data = await res.json();
        if (!stopped) onData(data);
      } catch (err) {
        if (!stopped && onError) onError(err);
      }
    }
    void tick();
    const timer = window.setInterval(() => void tick(), ms);
    return {
      stop() {
        stopped = true;
        window.clearInterval(timer);
      }
    };
  }

  // src/boot.snippet.js
  var bootSnippet = "(function(){try{var t=localStorage.getItem('webkit-theme')||'light';document.documentElement.setAttribute('data-theme',t);var s=parseInt(localStorage.getItem('webkit-size'),10);if(!isNaN(s)){s=Math.max(12,Math.min(24,s));document.documentElement.style.fontSize=s+'px';}}catch(e){}})();";

  // src/webkit.ts
  var THEME_KEY = "webkit-theme";
  var FONT_KEY = "webkit-font";
  var SIZE_KEY = "webkit-size";
  var FIXATION_KEY = "webkit-fixation";
  var SPEED_KEY = "webkit-ra-speed";
  var FONT_STACKS = {
    "Fira Code": "'Fira Code', 'SF Mono', 'Cascadia Code', monospace",
    "Inter": "'Inter', 'Helvetica Neue', Arial, sans-serif",
    "Lexend": "'Lexend', 'Helvetica Neue', Arial, sans-serif",
    "Work Sans": "'Work Sans', 'Helvetica Neue', Arial, sans-serif"
  };
  var DEFAULT_FIXATION_TARGETS = "[data-fixation], wk-panel-title, wk-panel-subtitle, wk-card, .callout, main p, main li, main td";
  var SVG_FIXATION = `<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M4 7V4h16v3"/><path d="M9 20h6"/><path d="M12 4v16"/></svg>`;
  var SVG_SUN = `<svg class="icon-sun" viewBox="0 0 24 24" fill="currentColor"><path d="M12 7c-2.76 0-5 2.24-5 5s2.24 5 5 5 5-2.24 5-5-2.24-5-5-5zM2 13h2c.55 0 1-.45 1-1s-.45-1-1-1H2c-.55 0-1 .45-1 1s.45 1 1 1zm18 0h2c.55 0 1-.45 1-1s-.45-1-1-1h-2c-.55 0-1 .45-1 1s.45 1 1 1zM11 2v2c0 .55.45 1 1 1s1-.45 1-1V2c0-.55-.45-1-1-1s-1 .45-1 1zm0 18v2c0 .55.45 1 1 1s1-.45 1-1v-2c0-.55-.45-1-1-1s-1 .45-1 1zM5.99 4.58a.996.996 0 00-1.41 0 .996.996 0 000 1.41l1.06 1.06c.39.39 1.03.39 1.41 0s.39-1.03 0-1.41L5.99 4.58zm12.37 12.37a.996.996 0 00-1.41 0 .996.996 0 000 1.41l1.06 1.06c.39.39 1.03.39 1.41 0a.996.996 0 000-1.41l-1.06-1.06zm1.06-10.96a.996.996 0 000-1.41.996.996 0 00-1.41 0l-1.06 1.06c-.39.39-.39 1.03 0 1.41s1.03.39 1.41 0l1.06-1.06zM7.05 18.36a.996.996 0 000-1.41.996.996 0 00-1.41 0l-1.06 1.06c-.39.39-.39 1.03 0 1.41s1.03.39 1.41 0l1.06-1.06z"/></svg>`;
  var SVG_MOON = `<svg class="icon-moon" viewBox="0 0 24 24" fill="currentColor"><path d="M12 3a9 9 0 109 9c0-.46-.04-.92-.1-1.36a5.389 5.389 0 01-4.4 2.26 5.403 5.403 0 01-3.14-9.8c-.44-.06-.9-.1-1.36-.1z"/></svg>`;
  var SVG_RELOAD = `<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M23 4v6h-6"/><path d="M1 20v-6h6"/><path d="M3.51 9a9 9 0 0114.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0020.49 15"/></svg>`;
  var SVG_REFRESH = `<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M23 4v6h-6"/><path d="M1 20v-6h6"/><path d="M3.51 9a9 9 0 0114.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0020.49 15"/></svg>`;
  var SVG_ADD = `<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M12 5v14"/><path d="M5 12h14"/></svg>`;
  var SVG_SEARCH = `<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="7"/><path d="M21 21l-4.3-4.3"/></svg>`;
  var SVG_HELP = `<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M8.6 8.5a3.4 3.4 0 0 1 6.6 1.1c0 2.2-3.2 2.9-3.2 5"/><path d="M12 18.5h.01"/></svg>`;
  var SVG_CLOSE = `<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 6L6 18"/><path d="M6 6l12 12"/></svg>`;
  var SPEED_OPTIONS = [
    { value: "0.75", label: "0.75\xD7" },
    { value: "1", label: "1\xD7" },
    { value: "1.25", label: "1.25\xD7" },
    { value: "1.5", label: "1.5\xD7" },
    { value: "2", label: "2\xD7" }
  ];
  function walkText(node) {
    if (node.nodeType === Node.TEXT_NODE) {
      const span = document.createElement("span");
      span.innerHTML = toFixation(node.textContent ?? "");
      node.parentNode?.replaceChild(span, node);
    } else if (node.nodeType === Node.ELEMENT_NODE && !["CODE", "B", "STRONG", "SCRIPT", "STYLE"].includes(node.tagName)) {
      Array.from(node.childNodes).forEach(walkText);
    }
  }
  function renderBrand(text, href) {
    const last = text[text.length - 1];
    let inner;
    if (last === "." || last === "\xB7") {
      inner = escHtml(text.slice(0, -1)) + `<span class="dot">${escHtml(last)}</span>`;
    } else {
      inner = escHtml(text);
    }
    return `<a class="topbar-brand" href="${escAttr(href)}">${inner}</a>`;
  }
  function escHtml(s) {
    return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
  }
  function escAttr(s) {
    return s.replace(/&/g, "&amp;").replace(/"/g, "&quot;");
  }
  function iconSvg(icon) {
    return icon === "refresh" ? SVG_REFRESH : SVG_ADD;
  }
  function applyFont(name, selectEl) {
    if (!(name in FONT_STACKS)) name = "Fira Code";
    document.body.style.fontFamily = FONT_STACKS[name];
    selectEl.value = name;
    localStorage.setItem(FONT_KEY, name);
  }
  function applySize(px) {
    const clamped = clampSize(px);
    document.documentElement.style.fontSize = clamped + "px";
    localStorage.setItem(SIZE_KEY, String(clamped));
    return clamped;
  }
  var WkHeader = class extends HTMLElement {
    constructor() {
      super(...arguments);
      this._initialized = false;
    }
    connectedCallback() {
      if (this._initialized) return;
      this._render();
      this._applyTheme();
      this._wire();
      this._initialized = true;
    }
    _attr(name, fallback = "") {
      return this.getAttribute(name) ?? fallback;
    }
    _controls() {
      const raw = this._attr("controls", "cmdk,font,fixation,size,reload,theme,help");
      return raw.split(",").map((s) => s.trim()).filter(Boolean);
    }
    /** Whether this header lists the ⌘K site picker; the keyboard shortcut follows it. */
    wantsCmdK() {
      return this._controls().includes("cmdk");
    }
    _render() {
      const brand = this._attr("brand");
      const brandHref = this._attr("brand-href", "/");
      const backLabel = this._attr("back-label");
      const backHref = this._attr("back-href");
      const title = this._attr("title");
      const pageWidth = parseInt(this._attr("page-width", "1080"), 10) || 1080;
      document.documentElement.style.setProperty("--page-width", pageWidth + "px");
      const navChildren = Array.from(this.querySelectorAll("[data-nav]"));
      const extraChildren = Array.from(this.querySelectorAll("[data-extra]"));
      const leftParts = [];
      if (backLabel) {
        leftParts.push(`<a class="topbar-link" href="${escAttr(backHref)}">${escHtml(backLabel)}</a>`);
      }
      if (brand) {
        leftParts.push(renderBrand(brand, brandHref));
      }
      if (title) {
        leftParts.push(`<span class="topbar-title">${escHtml(title)}</span>`);
      }
      const navHtml = navChildren.map((a) => a.outerHTML).join("");
      if (navHtml) {
        leftParts.push(`<nav class="topbar-nav">${navHtml}</nav>`);
      }
      const controlParts = [];
      let needsDivider = false;
      function divider() {
        if (needsDivider) {
          needsDivider = false;
          return '<span class="ctrl-divider"></span>';
        }
        return "";
      }
      for (const control of this._controls()) {
        switch (control) {
          case "cmdk":
            controlParts.push(divider());
            controlParts.push(
              `<button class="ctrl-btn" id="webkit-cmdk" aria-label="Jump to site" title="Jump to site (\u2318K)">` + SVG_SEARCH + ` <kbd class="ctrl-kbd" data-cmdk-kbd>\u2318K</kbd></button>`
            );
            needsDivider = true;
            break;
          case "font":
            controlParts.push(divider());
            controlParts.push(
              `<select class="ctrl-select" id="webkit-font" aria-label="Font family">` + Object.keys(FONT_STACKS).map((k) => `<option value="${escAttr(k)}">${escHtml(k)}</option>`).join("") + `</select>`
            );
            needsDivider = true;
            break;
          case "fixation":
            controlParts.push(divider());
            controlParts.push(
              `<button class="ctrl-btn" id="webkit-fixation" aria-label="Toggle fixation reading" aria-pressed="false">` + SVG_FIXATION + ` Fixation</button>`
            );
            needsDivider = true;
            break;
          case "size":
            controlParts.push(divider());
            controlParts.push(
              `<div class="ctrl-group"><button class="size-btn" id="webkit-size-down" aria-label="Decrease font size">&minus;</button><button class="size-btn" id="webkit-size-up" aria-label="Increase font size">+</button></div>`
            );
            needsDivider = true;
            break;
          case "speed":
            controlParts.push(divider());
            controlParts.push(
              `<select class="ctrl-select" id="webkit-ra-speed" aria-label="Speech speed">` + SPEED_OPTIONS.map((o) => `<option value="${escAttr(o.value)}">${escHtml(o.label)}</option>`).join("") + `</select>`
            );
            needsDivider = true;
            break;
          case "reload":
            controlParts.push(divider());
            controlParts.push(
              `<button class="ctrl-btn" id="webkit-reload" aria-label="Reload page">` + SVG_RELOAD + ` Reload</button>`
            );
            needsDivider = true;
            break;
          case "theme":
            controlParts.push(divider());
            controlParts.push(
              `<button class="theme-toggle" id="webkit-theme" aria-label="Switch to dark mode" aria-pressed="false">` + SVG_SUN + SVG_MOON + `</button>`
            );
            needsDivider = false;
            break;
          case "help":
            controlParts.push('<span class="ctrl-divider"></span>');
            controlParts.push(
              `<button class="ctrl-btn" id="webkit-help" aria-label="Feature guide" aria-haspopup="dialog" title="Feature guide">` + SVG_HELP + `</button>`
            );
            needsDivider = false;
            break;
        }
      }
      if (extraChildren.length > 0) {
        if (needsDivider) controlParts.push('<span class="ctrl-divider"></span>');
        for (const ex of extraChildren) {
          controlParts.push(ex.outerHTML);
        }
      }
      this.innerHTML = `<div class="topbar"><div class="topbar-inner"><div class="topbar-left">${leftParts.join("")}</div><div class="topbar-controls">${controlParts.join("")}</div></div></div>`;
    }
    _applyTheme() {
      const saved = localStorage.getItem(THEME_KEY) ?? "light";
      document.documentElement.setAttribute("data-theme", saved);
    }
    _wire() {
      const fixationTargets = this.getAttribute("fixation-targets") ?? DEFAULT_FIXATION_TARGETS;
      const fontEl = this.querySelector("#webkit-font");
      if (fontEl) {
        const savedFont = localStorage.getItem(FONT_KEY) ?? "Fira Code";
        applyFont(savedFont, fontEl);
        fontEl.addEventListener("change", () => applyFont(fontEl.value, fontEl));
      }
      let currentSize = parseInt(localStorage.getItem(SIZE_KEY) ?? "", 10);
      if (isNaN(currentSize)) currentSize = SIZE_DEFAULT;
      applySize(currentSize);
      this.querySelector("#webkit-size-down")?.addEventListener("click", () => {
        currentSize = applySize(currentSize - SIZE_STEP);
      });
      this.querySelector("#webkit-size-up")?.addEventListener("click", () => {
        currentSize = applySize(currentSize + SIZE_STEP);
      });
      const speedEl = this.querySelector("#webkit-ra-speed");
      if (speedEl) {
        const savedSpeed = localStorage.getItem(SPEED_KEY) ?? "1";
        speedEl.value = savedSpeed;
        speedEl.addEventListener("change", () => {
          localStorage.setItem(SPEED_KEY, speedEl.value);
          document.dispatchEvent(new CustomEvent("wk-speedchange", { detail: { speed: parseFloat(speedEl.value) } }));
        });
      }
      const fixationBtn = this.querySelector("#webkit-fixation");
      const fixationOriginals = /* @__PURE__ */ new WeakMap();
      let fixationActive = localStorage.getItem(FIXATION_KEY) === "true";
      const fixationify = (el2) => {
        if (el2.dataset.fixationDone === "1") return;
        fixationOriginals.set(el2, el2.innerHTML);
        el2.classList.add("fixation");
        const frag = document.createElement("div");
        frag.innerHTML = el2.innerHTML;
        walkText(frag);
        el2.innerHTML = frag.innerHTML;
        el2.dataset.fixationDone = "1";
      };
      const unbionify = (el2) => {
        if (el2.dataset.fixationDone !== "1") return;
        const orig = fixationOriginals.get(el2);
        if (orig !== void 0) el2.innerHTML = orig;
        el2.classList.remove("fixation");
        delete el2.dataset.fixationDone;
      };
      const applyFixation = (active) => {
        fixationBtn?.classList.toggle("active", active);
        fixationBtn?.setAttribute("aria-pressed", String(active));
        document.querySelectorAll(fixationTargets).forEach((el2) => {
          if (active) fixationify(el2);
          else unbionify(el2);
        });
      };
      let fixationScheduled = false;
      const observe = () => fixationObserver.observe(document.body, { childList: true, subtree: true });
      const fixationObserver = new MutationObserver(() => {
        if (fixationScheduled || !fixationActive) return;
        fixationScheduled = true;
        requestAnimationFrame(() => {
          fixationScheduled = false;
          if (!fixationActive) return;
          fixationObserver.disconnect();
          applyFixation(true);
          observe();
        });
      });
      applyFixation(fixationActive);
      if (fixationActive) observe();
      fixationBtn?.addEventListener("click", () => {
        stopReadAloud();
        fixationActive = !fixationActive;
        localStorage.setItem(FIXATION_KEY, String(fixationActive));
        fixationObserver.disconnect();
        applyFixation(fixationActive);
        if (fixationActive) observe();
      });
      this.querySelector("#webkit-reload")?.addEventListener("click", () => {
        location.reload();
      });
      const controls = this._controls();
      this.querySelector("#webkit-help")?.addEventListener("click", () => openHelp(controls));
      const cmdkBtn = this.querySelector("#webkit-cmdk");
      if (cmdkBtn) {
        const kbd = cmdkBtn.querySelector("[data-cmdk-kbd]");
        const isMac = /Mac|iP(hone|ad|od)/.test(navigator.platform || navigator.userAgent);
        if (kbd && !isMac) kbd.textContent = "Ctrl K";
        cmdkBtn.addEventListener("click", () => {
          document.dispatchEvent(new CustomEvent("wk-cmdk:open"));
        });
      }
      const themeBtn = this.querySelector("#webkit-theme");
      const reflectTheme = (theme) => {
        themeBtn?.setAttribute("aria-pressed", String(theme === "dark"));
        themeBtn?.setAttribute(
          "aria-label",
          theme === "dark" ? "Switch to light mode" : "Switch to dark mode"
        );
      };
      reflectTheme(document.documentElement.getAttribute("data-theme") ?? "light");
      themeBtn?.addEventListener("click", () => {
        const current = document.documentElement.getAttribute("data-theme");
        const next = current === "dark" ? "light" : "dark";
        document.documentElement.setAttribute("data-theme", next);
        localStorage.setItem(THEME_KEY, next);
        reflectTheme(next);
        document.dispatchEvent(new CustomEvent("wk-themechange", { detail: { theme: next } }));
      });
    }
  };
  customElements.define("wk-header", WkHeader);
  customElements.define("wk-read-aloud", WkReadAloud);
  var NOOP_ELEMENTS = [
    // base kit
    "wk-page-header",
    "wk-title",
    "wk-subtitle",
    "wk-card",
    "wk-panel",
    "wk-panel-title",
    "wk-panel-subtitle",
    "wk-badge",
    "wk-button",
    "wk-search",
    "wk-table",
    // segmented control
    "wk-seg",
    // key/value rows
    "wk-kv",
    "wk-kv-row",
    "wk-kv-label",
    "wk-kv-value",
    // section block
    "wk-section",
    "wk-section-heading",
    "wk-section-id",
    "wk-section-subheading",
    // callout
    "wk-callout",
    // progress bar
    "wk-progress",
    "wk-progress-bar",
    "wk-progress-fill",
    "wk-progress-label",
    // table of contents
    "wk-toc",
    "wk-toc-title",
    // modal + form
    "wk-modal",
    "wk-modal-panel",
    "wk-modal-head",
    "wk-modal-body",
    "wk-modal-actions",
    "wk-form",
    "wk-form-row",
    "wk-form-hint",
    // toasts
    "wk-toast",
    "wk-toast-host"
  ];
  for (const tag of NOOP_ELEMENTS) {
    customElements.define(tag, class extends HTMLElement {
    });
  }
  var CMDK_SITES_PATH = "/__this/sites.json";
  var CMDK_SITES_FALLBACK = "http://present.this" + CMDK_SITES_PATH;
  var cmdkSites = null;
  var cmdkInFlight = null;
  var cmdkOverlay = null;
  var cmdkInput = null;
  var cmdkListEl = null;
  var cmdkMatches = [];
  var cmdkActive = 0;
  var cmdkPrevFocus = null;
  async function cmdkLoadSites() {
    if (cmdkSites) return cmdkSites;
    if (!cmdkInFlight) {
      cmdkInFlight = (async () => {
        for (const url of [CMDK_SITES_PATH, CMDK_SITES_FALLBACK]) {
          try {
            const res = await fetch(url, { credentials: "omit" });
            if (!res.ok) continue;
            const data = await res.json();
            if (Array.isArray(data) && data.length) {
              cmdkSites = data;
              return cmdkSites;
            }
          } catch {
          }
        }
        cmdkInFlight = null;
        return [];
      })();
    }
    return cmdkInFlight;
  }
  function cmdkHostOf(url) {
    try {
      return new URL(url).host;
    } catch {
      return url;
    }
  }
  function cmdkHighlight(name, indices) {
    if (!indices.length) return escHtml(name);
    const hit = new Set(indices);
    let out = "";
    for (let i = 0; i < name.length; i++) {
      const ch = escHtml(name[i]);
      out += hit.has(i) ? `<mark>${ch}</mark>` : ch;
    }
    return out;
  }
  function cmdkIsOpen() {
    return !!cmdkOverlay && !cmdkOverlay.hasAttribute("hidden");
  }
  function cmdkBuild() {
    if (cmdkOverlay) return;
    const overlay = document.createElement("div");
    overlay.className = "wk-cmdk-overlay";
    overlay.setAttribute("hidden", "");
    overlay.setAttribute("role", "dialog");
    overlay.setAttribute("aria-modal", "true");
    overlay.setAttribute("aria-label", "Jump to site");
    overlay.innerHTML = `<div class="wk-cmdk-panel"><input class="wk-cmdk-input" type="text" placeholder="Jump to site\u2026" autocomplete="off" autocorrect="off" autocapitalize="off" spellcheck="false" aria-label="Jump to site"><div class="wk-cmdk-list" role="listbox"></div></div>`;
    document.body.appendChild(overlay);
    cmdkOverlay = overlay;
    cmdkInput = overlay.querySelector(".wk-cmdk-input");
    cmdkListEl = overlay.querySelector(".wk-cmdk-list");
    overlay.addEventListener("mousedown", (e) => {
      if (e.target === overlay) cmdkClose();
    });
    cmdkInput?.addEventListener("input", () => cmdkRender());
    cmdkInput?.addEventListener("keydown", cmdkOnKey);
  }
  function cmdkRender() {
    if (!cmdkListEl || !cmdkInput) return;
    cmdkMatches = rankSites(cmdkInput.value, cmdkSites ?? []);
    cmdkActive = 0;
    if (cmdkMatches.length === 0) {
      const msg = cmdkSites && cmdkSites.length ? "No matching site" : "No sites found";
      cmdkListEl.innerHTML = `<div class="wk-cmdk-empty">${msg}</div>`;
      return;
    }
    cmdkListEl.innerHTML = cmdkMatches.map(
      (m, i) => `<div class="wk-cmdk-item${i === 0 ? " active" : ""}" role="option" data-i="${i}" aria-selected="${i === 0}"><span class="wk-cmdk-name">${cmdkHighlight(m.site.name, m.indices)}</span><span class="wk-cmdk-host">${escHtml(m.site.host ?? cmdkHostOf(m.site.url))}</span></div>`
    ).join("");
    cmdkListEl.querySelectorAll(".wk-cmdk-item").forEach((el2) => {
      el2.addEventListener("click", () => {
        cmdkActive = parseInt(el2.dataset.i ?? "0", 10);
        cmdkCommit();
      });
      el2.addEventListener("mousemove", () => {
        const i = parseInt(el2.dataset.i ?? "0", 10);
        if (i !== cmdkActive) {
          cmdkActive = i;
          cmdkReflect();
        }
      });
    });
  }
  function cmdkReflect() {
    cmdkListEl?.querySelectorAll(".wk-cmdk-item").forEach((el2, i) => {
      const on = i === cmdkActive;
      el2.classList.toggle("active", on);
      el2.setAttribute("aria-selected", String(on));
      if (on) el2.scrollIntoView({ block: "nearest" });
    });
  }
  function cmdkMove(delta) {
    if (cmdkMatches.length === 0) return;
    cmdkActive = (cmdkActive + delta + cmdkMatches.length) % cmdkMatches.length;
    cmdkReflect();
  }
  function cmdkCommit() {
    const m = cmdkMatches[cmdkActive];
    if (!m) return;
    cmdkClose();
    window.location.href = m.site.url;
  }
  function cmdkOnKey(e) {
    switch (e.key) {
      case "ArrowDown":
        e.preventDefault();
        cmdkMove(1);
        break;
      case "ArrowUp":
        e.preventDefault();
        cmdkMove(-1);
        break;
      case "Enter":
        e.preventDefault();
        cmdkCommit();
        break;
      case "Escape":
        e.preventDefault();
        cmdkClose();
        break;
      case "n":
        if (e.ctrlKey) {
          e.preventDefault();
          cmdkMove(1);
        }
        break;
      // vim-ish
      case "p":
        if (e.ctrlKey) {
          e.preventDefault();
          cmdkMove(-1);
        }
        break;
    }
  }
  async function cmdkOpen() {
    cmdkBuild();
    if (!cmdkOverlay || !cmdkInput || !cmdkListEl || cmdkIsOpen()) return;
    cmdkPrevFocus = document.activeElement;
    cmdkOverlay.removeAttribute("hidden");
    cmdkInput.value = "";
    cmdkListEl.innerHTML = `<div class="wk-cmdk-empty">Loading\u2026</div>`;
    cmdkInput.focus();
    await cmdkLoadSites();
    if (cmdkIsOpen()) cmdkRender();
  }
  function cmdkClose() {
    if (!cmdkOverlay) return;
    cmdkOverlay.setAttribute("hidden", "");
    if (cmdkPrevFocus instanceof HTMLElement) cmdkPrevFocus.focus();
    cmdkPrevFocus = null;
  }
  function openCmdK() {
    void cmdkOpen();
  }
  function cmdkWanted() {
    const headers = Array.from(document.querySelectorAll("wk-header"));
    return headers.length === 0 || headers.some((h) => h instanceof WkHeader && h.wantsCmdK());
  }
  var cmdkInstalled = false;
  function initCmdK() {
    if (cmdkInstalled) return;
    cmdkInstalled = true;
    document.addEventListener("keydown", (e) => {
      if ((e.metaKey || e.ctrlKey) && (e.key === "k" || e.key === "K")) {
        if (!cmdkWanted()) return;
        e.preventDefault();
        if (cmdkIsOpen()) cmdkClose();
        else void cmdkOpen();
      }
    });
    document.addEventListener("wk-cmdk:open", () => {
      void cmdkOpen();
    });
  }
  initCmdK();
  var CONTROL_HELP = {
    cmdk: { term: "\u2318K / Ctrl-K", desc: "Jump to any .this tool \u2014 fuzzy-search the site list and press Enter." },
    font: { term: "Font", desc: "Switch typeface. Lexend and Work Sans are tuned for easier reading." },
    size: { term: "\u2212 / +", desc: "Shrink or enlarge the text. Your size is remembered across pages." },
    fixation: { term: "Fixation", desc: "Bolds the first half of every word so your eyes anchor on each one \u2014 a reading aid that helps many dyslexic readers move through text faster." },
    speed: { term: "Speed", desc: "Playback speed for read-aloud \u2014 steps through 0.75\xD7 \xB7 1\xD7 \xB7 1.25\xD7 \xB7 1.5\xD7 \xB7 2\xD7." },
    reload: { term: "Reload", desc: "Reload the page." },
    theme: { term: "Theme", desc: "Toggle light and dark." }
  };
  var HELP_ORDER = ["cmdk", "font", "size", "fixation", "speed", "reload", "theme"];
  var helpOverlay = null;
  var helpPrevFocus = null;
  function helpIsOpen() {
    return !!helpOverlay && !helpOverlay.hasAttribute("hidden");
  }
  function helpRowsHtml(controls) {
    const present = new Set(controls);
    const rows = [];
    for (const c of HELP_ORDER) {
      const h = present.has(c) ? CONTROL_HELP[c] : void 0;
      if (!h) continue;
      rows.push(
        `<div class="wk-help-row"><div class="wk-help-term">${escHtml(h.term)}</div><div class="wk-help-desc">${escHtml(h.desc)}</div></div>`
      );
    }
    return rows.join("");
  }
  function helpReadAloudHtml() {
    return `<div class="wk-help-section-title">Read aloud</div><div class="wk-help-row"><div class="wk-help-term">Play a section</div><div class="wk-help-desc">Each section gets a play button. It reads the text sentence by sentence and highlights the current one, so you can follow along or listen hands-free \u2014 a strong pairing with fixation for getting through long pages.</div></div><div class="wk-help-row"><div class="wk-help-term">Speed ladder</div><div class="wk-help-desc">The speed selector steps 0.75\xD7 \xB7 1\xD7 \xB7 1.25\xD7 \xB7 1.5\xD7 \xB7 2\xD7. A change applies from the next sentence and is remembered across pages.</div></div>`;
  }
  function helpExtraHtml() {
    let out = "";
    document.querySelectorAll("template[data-wk-help]").forEach((t) => {
      out += t.innerHTML;
    });
    return out;
  }
  function helpBuild() {
    if (helpOverlay) return;
    const overlay = document.createElement("wk-modal");
    overlay.setAttribute("hidden", "");
    overlay.setAttribute("role", "dialog");
    overlay.setAttribute("aria-modal", "true");
    overlay.setAttribute("aria-label", "Feature guide");
    overlay.innerHTML = `<wk-modal-panel><wk-modal-head><h3>Feature guide</h3><button class="wk-help-close" aria-label="Close guide">${SVG_CLOSE}</button></wk-modal-head><wk-modal-body></wk-modal-body></wk-modal-panel>`;
    document.body.appendChild(overlay);
    helpOverlay = overlay;
    overlay.addEventListener("mousedown", (e) => {
      if (e.target === overlay) closeHelp();
    });
    overlay.querySelector(".wk-help-close")?.addEventListener("click", () => closeHelp());
  }
  function openHelp(controls) {
    helpBuild();
    if (!helpOverlay || helpIsOpen()) return;
    const body = helpOverlay.querySelector("wk-modal-body");
    if (body) {
      body.innerHTML = helpRowsHtml(controls) + (controls.includes("speed") ? helpReadAloudHtml() : "") + helpExtraHtml();
    }
    helpPrevFocus = document.activeElement;
    helpOverlay.removeAttribute("hidden");
    helpOverlay.querySelector(".wk-help-close")?.focus();
  }
  function closeHelp() {
    if (!helpOverlay) return;
    helpOverlay.setAttribute("hidden", "");
    if (helpPrevFocus instanceof HTMLElement) helpPrevFocus.focus();
    helpPrevFocus = null;
  }
  function openFeatureGuide(controls = ["cmdk", "font", "size", "fixation", "reload", "theme"]) {
    openHelp(controls);
  }
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && helpIsOpen()) {
      e.preventDefault();
      closeHelp();
    }
  });
  var VERSION_URL = "/webkit/version";
  var VERSION_POLL_MS = 5e3;
  var seenVersion = null;
  async function pollVersion() {
    try {
      const res = await fetch(VERSION_URL, { cache: "no-store" });
      if (!res.ok) return;
      const data = await res.json();
      const v = typeof data?.version === "string" ? data.version : null;
      if (!v) return;
      if (seenVersion === null) {
        seenVersion = v;
      } else if (v !== seenVersion) {
        location.reload();
      }
    } catch {
    }
  }
  function initVersionPoll() {
    void pollVersion();
    setInterval(() => void pollVersion(), VERSION_POLL_MS);
  }
  initVersionPoll();
  initProse();
  function init(cfg) {
    const config = {
      mount: cfg?.mount ?? "#webkit-header",
      brand: cfg?.brand ?? "",
      brandHref: cfg?.brandHref ?? "/",
      back: cfg?.back ?? { label: "", href: "" },
      title: cfg?.title ?? "",
      nav: cfg?.nav ?? [],
      extra: cfg?.extra ?? [],
      controls: cfg?.controls ?? ["cmdk", "font", "fixation", "size", "reload", "theme", "help"],
      pageWidth: cfg?.pageWidth ?? 1080,
      fixationTargets: cfg?.fixationTargets ?? DEFAULT_FIXATION_TARGETS,
      onThemeChange: cfg?.onThemeChange ?? ((_t) => {
      })
    };
    const mountEl = document.querySelector(config.mount);
    if (!mountEl) return;
    const header = document.createElement("wk-header");
    if (config.brand) header.setAttribute("brand", config.brand);
    if (config.brandHref) header.setAttribute("brand-href", config.brandHref);
    if (config.back.label) header.setAttribute("back-label", config.back.label);
    if (config.back.href) header.setAttribute("back-href", config.back.href);
    if (config.title) header.setAttribute("title", config.title);
    header.setAttribute("controls", config.controls.join(","));
    header.setAttribute("page-width", String(config.pageWidth));
    if (config.fixationTargets) header.setAttribute("fixation-targets", config.fixationTargets);
    for (const item of config.nav) {
      const a = document.createElement("a");
      a.setAttribute("data-nav", "");
      a.href = item.href;
      a.textContent = item.label;
      if (item.active) a.classList.add("active");
      header.appendChild(a);
    }
    for (const ex of config.extra) {
      const btn = document.createElement("wk-button");
      btn.setAttribute("data-extra", "");
      btn.setAttribute("variant", ex.variant ?? "ghost");
      btn.setAttribute("role", "button");
      btn.setAttribute("tabindex", "0");
      btn.id = ex.id;
      if (ex.title) btn.title = ex.title;
      if (ex.icon) btn.innerHTML = iconSvg(ex.icon);
      if (ex.label) btn.append(document.createTextNode(ex.label));
      header.appendChild(btn);
    }
    if (cfg?.onThemeChange) {
      document.addEventListener("wk-themechange", ((e) => {
        cfg.onThemeChange(e.detail.theme);
      }));
    }
    mountEl.replaceWith(header);
  }
  return __toCommonJS(webkit_exports);
})();
