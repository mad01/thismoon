/* brain-graph bundle: @sigma/node-border 4.0.0, d3-force 3.0.1, graphology 0.26.0, graphology-communities-louvain 2.0.2, graphology-utils 2.5.2, markdown-it 15.0.3, sigma 4.0.4. Built by services/brain/web/build.mjs from web/package-lock.json; licences at the end of this file and in services/brain/THIRD-PARTY.md. Do not edit. */
(()=>{var mc=Object.create;var On=Object.defineProperty;var bc=Object.getOwnPropertyDescriptor;var yc=Object.getOwnPropertyNames;var _c=Object.getPrototypeOf,xc=Object.prototype.hasOwnProperty;var Tc=(e,t,n)=>t in e?On(e,t,{enumerable:!0,configurable:!0,writable:!0,value:n}):e[t]=n;var Ae=(e,t)=>()=>{try{return t||e((t={exports:{}}).exports,t),t.exports}catch(n){throw t=0,n}},Pi=(e,t)=>{for(var n in t)On(e,n,{get:t[n],enumerable:!0})},Ec=(e,t,n,r)=>{if(t&&typeof t=="object"||typeof t=="function")for(let a of yc(t))!xc.call(e,a)&&a!==n&&On(e,a,{get:()=>t[a],enumerable:!(r=bc(t,a))||r.enumerable});return e};var Ne=(e,t,n)=>(n=e!=null?mc(_c(e)):{},Ec(t||!e||!e.__esModule?On(n,"default",{value:e,enumerable:!0}):n,e));var ue=(e,t,n)=>Tc(e,typeof t!="symbol"?t+"":t,n);var dt=Ae((Qm,aa)=>{"use strict";var Dt=typeof Reflect=="object"?Reflect:null,Ii=Dt&&typeof Dt.apply=="function"?Dt.apply:function(t,n,r){return Function.prototype.apply.call(t,n,r)},Un;Dt&&typeof Dt.ownKeys=="function"?Un=Dt.ownKeys:Object.getOwnPropertySymbols?Un=function(t){return Object.getOwnPropertyNames(t).concat(Object.getOwnPropertySymbols(t))}:Un=function(t){return Object.getOwnPropertyNames(t)};function Sc(e){console&&console.warn&&console.warn(e)}var zi=Number.isNaN||function(t){return t!==t};function ee(){ee.init.call(this)}aa.exports=ee;aa.exports.once=Dc;ee.EventEmitter=ee;ee.prototype._events=void 0;ee.prototype._eventsCount=0;ee.prototype._maxListeners=void 0;var Ni=10;function Hn(e){if(typeof e!="function")throw new TypeError('The "listener" argument must be of type Function. Received type '+typeof e)}Object.defineProperty(ee,"defaultMaxListeners",{enumerable:!0,get:function(){return Ni},set:function(e){if(typeof e!="number"||e<0||zi(e))throw new RangeError('The value of "defaultMaxListeners" is out of range. It must be a non-negative number. Received '+e+".");Ni=e}});ee.init=function(){(this._events===void 0||this._events===Object.getPrototypeOf(this)._events)&&(this._events=Object.create(null),this._eventsCount=0),this._maxListeners=this._maxListeners||void 0};ee.prototype.setMaxListeners=function(t){if(typeof t!="number"||t<0||zi(t))throw new RangeError('The value of "n" is out of range. It must be a non-negative number. Received '+t+".");return this._maxListeners=t,this};function Mi(e){return e._maxListeners===void 0?ee.defaultMaxListeners:e._maxListeners}ee.prototype.getMaxListeners=function(){return Mi(this)};ee.prototype.emit=function(t){for(var n=[],r=1;r<arguments.length;r++)n.push(arguments[r]);var a=t==="error",i=this._events;if(i!==void 0)a=a&&i.error===void 0;else if(!a)return!1;if(a){var o;if(n.length>0&&(o=n[0]),o instanceof Error)throw o;var s=new Error("Unhandled error."+(o?" ("+o.message+")":""));throw s.context=o,s}var l=i[t];if(l===void 0)return!1;if(typeof l=="function")Ii(l,this,n);else for(var u=l.length,c=Ui(l,u),r=0;r<u;++r)Ii(c[r],this,n);return!0};function Gi(e,t,n,r){var a,i,o;if(Hn(n),i=e._events,i===void 0?(i=e._events=Object.create(null),e._eventsCount=0):(i.newListener!==void 0&&(e.emit("newListener",t,n.listener?n.listener:n),i=e._events),o=i[t]),o===void 0)o=i[t]=n,++e._eventsCount;else if(typeof o=="function"?o=i[t]=r?[n,o]:[o,n]:r?o.unshift(n):o.push(n),a=Mi(e),a>0&&o.length>a&&!o.warned){o.warned=!0;var s=new Error("Possible EventEmitter memory leak detected. "+o.length+" "+String(t)+" listeners added. Use emitter.setMaxListeners() to increase limit");s.name="MaxListenersExceededWarning",s.emitter=e,s.type=t,s.count=o.length,Sc(s)}return e}ee.prototype.addListener=function(t,n){return Gi(this,t,n,!1)};ee.prototype.on=ee.prototype.addListener;ee.prototype.prependListener=function(t,n){return Gi(this,t,n,!0)};function Rc(){if(!this.fired)return this.target.removeListener(this.type,this.wrapFn),this.fired=!0,arguments.length===0?this.listener.call(this.target):this.listener.apply(this.target,arguments)}function Bi(e,t,n){var r={fired:!1,wrapFn:void 0,target:e,type:t,listener:n},a=Rc.bind(r);return a.listener=n,r.wrapFn=a,a}ee.prototype.once=function(t,n){return Hn(n),this.on(t,Bi(this,t,n)),this};ee.prototype.prependOnceListener=function(t,n){return Hn(n),this.prependListener(t,Bi(this,t,n)),this};ee.prototype.removeListener=function(t,n){var r,a,i,o,s;if(Hn(n),a=this._events,a===void 0)return this;if(r=a[t],r===void 0)return this;if(r===n||r.listener===n)--this._eventsCount===0?this._events=Object.create(null):(delete a[t],a.removeListener&&this.emit("removeListener",t,r.listener||n));else if(typeof r!="function"){for(i=-1,o=r.length-1;o>=0;o--)if(r[o]===n||r[o].listener===n){s=r[o].listener,i=o;break}if(i<0)return this;i===0?r.shift():Ac(r,i),r.length===1&&(a[t]=r[0]),a.removeListener!==void 0&&this.emit("removeListener",t,s||n)}return this};ee.prototype.off=ee.prototype.removeListener;ee.prototype.removeAllListeners=function(t){var n,r,a;if(r=this._events,r===void 0)return this;if(r.removeListener===void 0)return arguments.length===0?(this._events=Object.create(null),this._eventsCount=0):r[t]!==void 0&&(--this._eventsCount===0?this._events=Object.create(null):delete r[t]),this;if(arguments.length===0){var i=Object.keys(r),o;for(a=0;a<i.length;++a)o=i[a],o!=="removeListener"&&this.removeAllListeners(o);return this.removeAllListeners("removeListener"),this._events=Object.create(null),this._eventsCount=0,this}if(n=r[t],typeof n=="function")this.removeListener(t,n);else if(n!==void 0)for(a=n.length-1;a>=0;a--)this.removeListener(t,n[a]);return this};function Wi(e,t,n){var r=e._events;if(r===void 0)return[];var a=r[t];return a===void 0?[]:typeof a=="function"?n?[a.listener||a]:[a]:n?wc(a):Ui(a,a.length)}ee.prototype.listeners=function(t){return Wi(this,t,!0)};ee.prototype.rawListeners=function(t){return Wi(this,t,!1)};ee.listenerCount=function(e,t){return typeof e.listenerCount=="function"?e.listenerCount(t):Oi.call(e,t)};ee.prototype.listenerCount=Oi;function Oi(e){var t=this._events;if(t!==void 0){var n=t[e];if(typeof n=="function")return 1;if(n!==void 0)return n.length}return 0}ee.prototype.eventNames=function(){return this._eventsCount>0?Un(this._events):[]};function Ui(e,t){for(var n=new Array(t),r=0;r<t;++r)n[r]=e[r];return n}function Ac(e,t){for(;t+1<e.length;t++)e[t]=e[t+1];e.pop()}function wc(e){for(var t=new Array(e.length),n=0;n<t.length;++n)t[n]=e[n].listener||e[n];return t}function Dc(e,t){return new Promise(function(n,r){function a(o){e.removeListener(t,i),r(o)}function i(){typeof e.removeListener=="function"&&e.removeListener("error",a),n([].slice.call(arguments))}Hi(e,t,i,{once:!0}),t!=="error"&&Cc(e,a,{once:!0})})}function Cc(e,t,n){typeof e.on=="function"&&Hi(e,"error",t,n)}function Hi(e,t,n,r){if(typeof e.on=="function")r.once?e.once(t,n):e.on(t,n);else if(typeof e.addEventListener=="function")e.addEventListener(t,function a(i){r.once&&e.removeEventListener(t,a),n(i)});else throw new TypeError('The "emitter" argument must be of type EventEmitter. Received type '+typeof e)}});var Bt=Ae((fb,ko)=>{ko.exports=function(t){return t!==null&&typeof t=="object"&&typeof t.addUndirectedEdgeWithKey=="function"&&typeof t.dropNode=="function"&&typeof t.multi=="boolean"}});var ri=Ae((py,yl)=>{function Ig(e){return!e||typeof e!="object"||typeof e=="function"||Array.isArray(e)||e instanceof Set||e instanceof Map||e instanceof RegExp||e instanceof Date}function bl(e,t){e=e||{};var n={};for(var r in t){var a=e[r],i=t[r];if(!Ig(i)){n[r]=bl(a,i);continue}a===void 0?n[r]=i:n[r]=a}return n}yl.exports=bl});var xl=Ae((vy,_l)=>{var Ng=Bt();_l.exports=function(t){if(!Ng(t))throw new Error("graphology-utils/infer-type: expecting a valid graphology instance.");var n=t.type;return n!=="mixed"?n:t.directedSize===0&&t.undirectedSize===0||t.directedSize>0&&t.undirectedSize>0?"mixed":t.directedSize>0?"directed":"undirected"}});var ai=Ae((my,Tl)=>{function Oe(e){if(typeof e!="function")throw new Error("obliterator/iterator: expecting a function!");this.next=e}typeof Symbol<"u"&&(Oe.prototype[Symbol.iterator]=function(){return this});Oe.of=function(){var e=arguments,t=e.length,n=0;return new Oe(function(){return n>=t?{done:!0}:{done:!1,value:e[n++]}})};Oe.empty=function(){var e=new Oe(function(){return{done:!0}});return e};Oe.fromSequence=function(e){var t=0,n=e.length;return new Oe(function(){return t>=n?{done:!0}:{done:!1,value:e[t++]}})};Oe.is=function(e){return e instanceof Oe?!0:typeof e=="object"&&e!==null&&typeof e.next=="function"};Tl.exports=Oe});var Gr=Ae(Ue=>{var zg=Math.pow(2,8)-1,Mg=Math.pow(2,16)-1,Gg=Math.pow(2,32)-1,Bg=Math.pow(2,7)-1,Wg=Math.pow(2,15)-1,Og=Math.pow(2,31)-1;Ue.getPointerArray=function(e){var t=e-1;if(t<=zg)return Uint8Array;if(t<=Mg)return Uint16Array;if(t<=Gg)return Uint32Array;throw new Error("mnemonist: Pointer Array of size > 4294967295 is not supported.")};Ue.getSignedPointerArray=function(e){var t=e-1;return t<=Bg?Int8Array:t<=Wg?Int16Array:t<=Og?Int32Array:Float64Array};Ue.getNumberType=function(e){return e===(e|0)?Math.sign(e)===-1?e<=127&&e>=-128?Int8Array:e<=32767&&e>=-32768?Int16Array:Int32Array:e<=255?Uint8Array:e<=65535?Uint16Array:Uint32Array:Float64Array};var Ug={Uint8Array:1,Int8Array:2,Uint16Array:3,Int16Array:4,Uint32Array:5,Int32Array:6,Float32Array:7,Float64Array:8};Ue.getMinimalRepresentation=function(e,t){var n=null,r=0,a,i,o,s,l;for(s=0,l=e.length;s<l;s++)o=t?t(e[s]):e[s],i=Ue.getNumberType(o),a=Ug[i.name],a>r&&(r=a,n=i);return n};Ue.isTypedArray=function(e){return typeof ArrayBuffer<"u"&&ArrayBuffer.isView(e)};Ue.concat=function(){var e=0,t,n,r;for(t=0,r=arguments.length;t<r;t++)e+=arguments[t].length;var a=new arguments[0].constructor(e);for(t=0,n=0;t<r;t++)a.set(arguments[t],n),n+=arguments[t].length;return a};Ue.indices=function(e){for(var t=Ue.getPointerArray(e),n=new t(e),r=0;r<e;r++)n[r]=r;return n}});var Sl=Ae((yy,El)=>{var ii=ai(),Hg=Gr().getPointerArray;function pe(e,t){arguments.length<2&&(t=e,e=Array);var n=Hg(t);this.size=0,this.length=t,this.dense=new n(t),this.sparse=new n(t),this.vals=new e(t)}pe.prototype.clear=function(){this.size=0};pe.prototype.has=function(e){var t=this.sparse[e];return t<this.size&&this.dense[t]===e};pe.prototype.get=function(e){var t=this.sparse[e];if(t<this.size&&this.dense[t]===e)return this.vals[t]};pe.prototype.set=function(e,t){var n=this.sparse[e];return n<this.size&&this.dense[n]===e?(this.vals[n]=t,this):(this.dense[this.size]=e,this.sparse[e]=this.size,this.vals[this.size]=t,this.size++,this)};pe.prototype.delete=function(e){var t=this.sparse[e];return t>=this.size||this.dense[t]!==e?!1:(t=this.dense[this.size-1],this.dense[this.sparse[e]]=t,this.sparse[t]=this.sparse[e],this.size--,!0)};pe.prototype.forEach=function(e,t){t=arguments.length>1?t:this;for(var n=0;n<this.size;n++)e.call(t,this.vals[n],this.dense[n])};pe.prototype.keys=function(){var e=this.size,t=this.dense,n=0;return new ii(function(){if(n<e){var r=t[n];return n++,{value:r}}return{done:!0}})};pe.prototype.values=function(){var e=this.size,t=this.vals,n=0;return new ii(function(){if(n<e){var r=t[n];return n++,{value:r}}return{done:!0}})};pe.prototype.entries=function(){var e=this.size,t=this.dense,n=this.vals,r=0;return new ii(function(){if(r<e){var a=[t[r],n[r]];return r++,{value:a}}return{done:!0}})};typeof Symbol<"u"&&(pe.prototype[Symbol.iterator]=pe.prototype.entries);pe.prototype.inspect=function(){for(var e=new Map,t=0;t<this.size;t++)e.set(this.dense[t],this.vals[t]);return Object.defineProperty(e,"constructor",{value:pe,enumerable:!1}),e.length=this.length,this.vals.constructor!==Array&&(e.type=this.vals.constructor.name),e};typeof Symbol<"u"&&(pe.prototype[Symbol.for("nodejs.util.inspect.custom")]=pe.prototype.inspect);El.exports=pe});var Al=Ae((_y,Rl)=>{var jg=ai(),$g=Gr().getPointerArray;function Re(e){var t=$g(e);this.start=0,this.size=0,this.capacity=e,this.dense=new t(e),this.sparse=new t(e)}Re.prototype.clear=function(){this.start=0,this.size=0};Re.prototype.has=function(e){if(this.size===0)return!1;var t=this.sparse[e],n=t<this.capacity&&t>=this.start&&t<this.start+this.size||t<(this.start+this.size)%this.capacity;return n&&this.dense[t]===e};Re.prototype.enqueue=function(e){var t=this.sparse[e];if(this.size!==0){var n=t<this.capacity&&t>=this.start&&t<this.start+this.size||t<(this.start+this.size)%this.capacity;if(n&&this.dense[t]===e)return this}return t=(this.start+this.size)%this.capacity,this.dense[t]=e,this.sparse[e]=t,this.size++,this};Re.prototype.dequeue=function(){if(this.size!==0){var e=this.start;this.size--,this.start++,this.start===this.capacity&&(this.start=0);var t=this.dense[e];return this.sparse[t]=this.capacity,t}};Re.prototype.forEach=function(e,t){t=arguments.length>1?t:this;for(var n=this.capacity,r=this.size,a=this.start,i=0;i<r;)e.call(t,this.dense[a],i,this),a++,i++,a===n&&(a=0)};Re.prototype.values=function(){var e=this.dense,t=this.capacity,n=this.size,r=this.start,a=0;return new jg(function(){if(a>=n)return{done:!0};var i=e[r];return r++,a++,r===t&&(r=0),{value:i,done:!1}})};typeof Symbol<"u"&&(Re.prototype[Symbol.iterator]=Re.prototype.values);Re.prototype.inspect=function(){var e=[];return this.forEach(function(t){e.push(t)}),Object.defineProperty(e,"constructor",{value:Re,enumerable:!1}),e.capacity=this.capacity,e};typeof Symbol<"u"&&(Re.prototype[Symbol.for("nodejs.util.inspect.custom")]=Re.prototype.inspect);Rl.exports=Re});var kl=Ae((xy,Cl)=>{function wl(e){return function(t){return typeof t!="number"&&(t=t.length),Math.floor(e()*t)}}var Dl=wl(Math.random);Dl.createRandomIndex=wl;Cl.exports=Dl});var Fl=Ae(Br=>{function Vg(e){return typeof e!="number"||isNaN(e)?1:e}function qg(e,t){var n={},r=function(o){return typeof o>"u"?t:o};typeof t=="function"&&(r=t);var a=function(o){return r(o[e])},i=function(){return r(void 0)};return typeof e=="string"?(n.fromAttributes=a,n.fromGraph=function(o,s){return a(o.getNodeAttributes(s))},n.fromEntry=function(o,s){return a(s)}):typeof e=="function"?(n.fromAttributes=function(){throw new Error("graphology-utils/getters/createNodeValueGetter: irrelevant usage.")},n.fromGraph=function(o,s){return r(e(s,o.getNodeAttributes(s)))},n.fromEntry=function(o,s){return r(e(o,s))}):(n.fromAttributes=i,n.fromGraph=i,n.fromEntry=i),n}function Ll(e,t){var n={},r=function(o){return typeof o>"u"?t:o};typeof t=="function"&&(r=t);var a=function(o){return r(o[e])},i=function(){return r(void 0)};return typeof e=="string"?(n.fromAttributes=a,n.fromGraph=function(o,s){return a(o.getEdgeAttributes(s))},n.fromEntry=function(o,s){return a(s)},n.fromPartialEntry=n.fromEntry,n.fromMinimalEntry=n.fromEntry):typeof e=="function"?(n.fromAttributes=function(){throw new Error("graphology-utils/getters/createEdgeValueGetter: irrelevant usage.")},n.fromGraph=function(o,s){var l=o.extremities(s);return r(e(s,o.getEdgeAttributes(s),l[0],l[1],o.getNodeAttributes(l[0]),o.getNodeAttributes(l[1]),o.isUndirected(s)))},n.fromEntry=function(o,s,l,u,c,d,f){return r(e(o,s,l,u,c,d,f))},n.fromPartialEntry=function(o,s,l,u){return r(e(o,s,l,u))},n.fromMinimalEntry=function(o,s){return r(e(o,s))}):(n.fromAttributes=i,n.fromGraph=i,n.fromEntry=i,n.fromMinimalEntry=i),n}Br.createNodeValueGetter=qg;Br.createEdgeValueGetter=Ll;Br.createEdgeWeightGetter=function(e){return Ll(e,Vg)}});var Ml=Ae(oi=>{var it=Gr(),Pl=ri(),Il=Fl().createEdgeWeightGetter,Nl=Symbol.for("nodejs.util.inspect.custom"),zl={getEdgeWeight:"weight",keepDendrogram:!1,resolution:1};function oe(e,t){t=Pl(t,zl);var n=t.resolution,r=Il(t.getEdgeWeight).fromEntry,a=(e.size-e.selfLoopCount)*2,i=it.getPointerArray(a),o=it.getPointerArray(e.order+1),s=t.getEdgeWeight?Float64Array:it.getPointerArray(e.size*2);this.C=e.order,this.M=0,this.E=a,this.U=0,this.resolution=n,this.level=0,this.graph=e,this.nodes=new Array(e.order),this.keepDendrogram=t.keepDendrogram,this.neighborhood=new o(a),this.weights=new s(a),this.loops=new s(e.order),this.starts=new i(e.order+1),this.belongings=new o(e.order),this.dendrogram=[],this.mapping=null,this.counts=new o(e.order),this.unused=new o(e.order),this.totalWeights=new s(e.order);var l={},u,c=0,d=0,f=this;e.forEachNode(function(h){f.nodes[c]=h,l[h]=c,d+=e.undirectedDegreeWithoutSelfLoops(h),f.starts[c]=d,f.belongings[c]=c,f.counts[c]=1,c++}),e.forEachEdge(function(h,g,m,v,p,b,y){if(u=r(h,g,m,v,p,b,y),m=l[m],v=l[v],f.M+=u,m===v)f.totalWeights[m]+=u*2,f.loops[m]=u*2;else{f.totalWeights[m]+=u,f.totalWeights[v]+=u;var _=--f.starts[m],x=--f.starts[v];f.neighborhood[_]=v,f.neighborhood[x]=m,f.weights[_]=u,f.weights[x]=u}}),this.starts[c]=this.E,this.keepDendrogram?this.dendrogram.push(this.belongings.slice()):this.mapping=this.belongings.slice()}oe.prototype.isolate=function(e,t){var n=this.belongings[e];if(this.counts[n]===1)return n;var r=this.unused[--this.U],a=this.loops[e];return this.totalWeights[n]-=t+a,this.totalWeights[r]+=t+a,this.belongings[e]=r,this.counts[n]--,this.counts[r]++,r};oe.prototype.move=function(e,t,n){var r=this.belongings[e],a=this.loops[e];this.totalWeights[r]-=t+a,this.totalWeights[n]+=t+a,this.belongings[e]=n;var i=this.counts[r]--===1;this.counts[n]++,i&&(this.unused[this.U++]=r)};oe.prototype.computeNodeDegree=function(e){var t,n,r,a=0;for(t=this.starts[e],n=this.starts[e+1];t<n;t++)r=this.weights[t],a+=r;return a};oe.prototype.expensiveIsolate=function(e){var t=this.computeNodeDegree(e);return this.isolate(e,t)};oe.prototype.expensiveMove=function(e,t){var n=this.computeNodeDegree(e);this.move(e,n,t)};oe.prototype.zoomOut=function(){var e=new Array(this.C-this.U),t={},n=this.nodes.length,r=0,a=0,i,o,s,l,u,c,d,f,h;for(i=0,s=this.C;i<s;i++)c=this.belongings[i],c in t||(t[c]=r,e[r]={adj:{},totalWeights:this.totalWeights[c],internalWeights:0},r++),this.belongings[i]=t[c];var g,m;if(this.keepDendrogram){for(g=this.dendrogram[this.level],m=new(it.getPointerArray(r))(n),i=0;i<n;i++)m[i]=this.belongings[g[i]];this.dendrogram.push(m)}else for(i=0;i<n;i++)this.mapping[i]=this.belongings[this.mapping[i]];for(i=0,s=this.C;i<s;i++)for(c=this.belongings[i],f=e[c],h=f.adj,f.internalWeights+=this.loops[i],o=this.starts[i],l=this.starts[i+1];o<l;o++){if(u=this.neighborhood[o],d=this.belongings[u],c===d){f.internalWeights+=this.weights[o];continue}d in h||(h[d]=0),h[d]+=this.weights[o]}for(this.C=r,u=0,c=0;c<r;c++){f=e[c],h=f.adj,c=+c,this.totalWeights[c]=f.totalWeights,this.loops[c]=f.internalWeights,this.counts[c]=1,this.starts[c]=u,this.belongings[c]=c;for(d in h)this.neighborhood[u]=+d,this.weights[u]=h[d],a++,u++}return this.starts[r]=a,this.E=a,this.U=0,this.level++,t};oe.prototype.modularity=function(){var e,t,n,r,a,i=0,o=this.M*2,s=new Float64Array(this.C);for(n=0;n<this.C;n++)for(e=this.belongings[n],s[e]+=this.loops[n],r=this.starts[n],a=this.starts[n+1];r<a;r++)t=this.belongings[this.neighborhood[r]],e===t&&(s[e]+=this.weights[r]);for(n=0;n<this.C;n++)i+=s[n]/o-Math.pow(this.totalWeights[n]/o,2)*this.resolution;return i};oe.prototype.delta=function(e,t,n,r){var a=this.M,i=this.totalWeights[r];return t+=this.loops[e],n/a-i*t*this.resolution/(2*a*a)};oe.prototype.deltaWithOwnCommunity=function(e,t,n,r){var a=this.M,i=this.totalWeights[r];return t+=this.loops[e],n/a-(i-t)*t*this.resolution/(2*a*a)};oe.prototype.fastDelta=function(e,t,n,r){var a=this.M,i=this.totalWeights[r];return t+=this.loops[e],n-t*i*this.resolution/(2*a)};oe.prototype.fastDeltaWithOwnCommunity=function(e,t,n,r){var a=this.M,i=this.totalWeights[r];return t+=this.loops[e],n-t*(i-t)*this.resolution/(2*a)};oe.prototype.bounds=function(e){return[this.starts[e],this.starts[e+1]]};oe.prototype.project=function(){var e=this,t={};return e.nodes.slice(0,this.C).forEach(function(n,r){t[n]=Array.from(e.neighborhood.slice(e.starts[r],e.starts[r+1])).map(function(a){return e.nodes[a]})}),t};oe.prototype.collect=function(e){arguments.length<1&&(e=this.level);var t={},n=this.keepDendrogram?this.dendrogram[e]:this.mapping,r,a;for(r=0,a=n.length;r<a;r++)t[this.nodes[r]]=n[r];return t};oe.prototype.assign=function(e,t){arguments.length<2&&(t=this.level);var n=this.keepDendrogram?this.dendrogram[t]:this.mapping,r,a;for(r=0,a=n.length;r<a;r++)this.graph.setNodeAttribute(this.nodes[r],e,n[r])};oe.prototype[Nl]=function(){var e={};Object.defineProperty(e,"constructor",{value:oe,enumerable:!1}),e.C=this.C,e.M=this.M,e.E=this.E,e.U=this.U,e.resolution=this.resolution,e.level=this.level,e.nodes=this.nodes,e.starts=this.starts.slice(0,e.C+1);var t=["neighborhood","weights"],n=["counts","loops","belongings","totalWeights"],r=this;return t.forEach(function(a){e[a]=r[a].slice(0,e.E)}),n.forEach(function(a){e[a]=r[a].slice(0,e.C)}),e.unused=this.unused.slice(0,this.U),this.keepDendrogram?e.dendrogram=this.dendrogram:e.mapping=this.mapping,e};function le(e,t){t=Pl(t,zl);var n=t.resolution,r=Il(t.getEdgeWeight).fromEntry,a=(e.size-e.selfLoopCount)*2,i=it.getPointerArray(a),o=it.getPointerArray(e.order+1),s=t.getEdgeWeight?Float64Array:it.getPointerArray(e.size*2);this.C=e.order,this.M=0,this.E=a,this.U=0,this.resolution=n,this.level=0,this.graph=e,this.nodes=new Array(e.order),this.keepDendrogram=t.keepDendrogram,this.neighborhood=new o(a),this.weights=new s(a),this.loops=new s(e.order),this.starts=new i(e.order+1),this.offsets=new i(e.order),this.belongings=new o(e.order),this.dendrogram=[],this.counts=new o(e.order),this.unused=new o(e.order),this.totalInWeights=new s(e.order),this.totalOutWeights=new s(e.order);var l={},u,c=0,d=0,f=this;e.forEachNode(function(h){f.nodes[c]=h,l[h]=c,d+=e.outDegreeWithoutSelfLoops(h),f.starts[c]=d,d+=e.inDegreeWithoutSelfLoops(h),f.offsets[c]=d,f.belongings[c]=c,f.counts[c]=1,c++}),e.forEachEdge(function(h,g,m,v,p,b,y){if(u=r(h,g,m,v,p,b,y),m=l[m],v=l[v],f.M+=u,m===v)f.loops[m]+=u,f.totalInWeights[m]+=u,f.totalOutWeights[m]+=u;else{f.totalOutWeights[m]+=u,f.totalInWeights[v]+=u;var _=--f.starts[m],x=--f.offsets[v];f.neighborhood[_]=v,f.neighborhood[x]=m,f.weights[_]=u,f.weights[x]=u}}),this.starts[c]=this.E,this.keepDendrogram?this.dendrogram.push(this.belongings.slice()):this.mapping=this.belongings.slice()}le.prototype.bounds=oe.prototype.bounds;le.prototype.inBounds=function(e){return[this.offsets[e],this.starts[e+1]]};le.prototype.outBounds=function(e){return[this.starts[e],this.offsets[e]]};le.prototype.project=oe.prototype.project;le.prototype.projectIn=function(){var e=this,t={};return e.nodes.slice(0,this.C).forEach(function(n,r){t[n]=Array.from(e.neighborhood.slice(e.offsets[r],e.starts[r+1])).map(function(a){return e.nodes[a]})}),t};le.prototype.projectOut=function(){var e=this,t={};return e.nodes.slice(0,this.C).forEach(function(n,r){t[n]=Array.from(e.neighborhood.slice(e.starts[r],e.offsets[r])).map(function(a){return e.nodes[a]})}),t};le.prototype.isolate=function(e,t,n){var r=this.belongings[e];if(this.counts[r]===1)return r;var a=this.unused[--this.U],i=this.loops[e];return this.totalInWeights[r]-=t+i,this.totalInWeights[a]+=t+i,this.totalOutWeights[r]-=n+i,this.totalOutWeights[a]+=n+i,this.belongings[e]=a,this.counts[r]--,this.counts[a]++,a};le.prototype.move=function(e,t,n,r){var a=this.belongings[e],i=this.loops[e];this.totalInWeights[a]-=t+i,this.totalInWeights[r]+=t+i,this.totalOutWeights[a]-=n+i,this.totalOutWeights[r]+=n+i,this.belongings[e]=r;var o=this.counts[a]--===1;this.counts[r]++,o&&(this.unused[this.U++]=a)};le.prototype.computeNodeInDegree=function(e){var t,n,r,a=0;for(t=this.offsets[e],n=this.starts[e+1];t<n;t++)r=this.weights[t],a+=r;return a};le.prototype.computeNodeOutDegree=function(e){var t,n,r,a=0;for(t=this.starts[e],n=this.offsets[e];t<n;t++)r=this.weights[t],a+=r;return a};le.prototype.expensiveMove=function(e,t){var n=this.computeNodeInDegree(e),r=this.computeNodeOutDegree(e);this.move(e,n,r,t)};le.prototype.zoomOut=function(){var e=new Array(this.C-this.U),t={},n=this.nodes.length,r=0,a=0,i,o,s,l,u,c,d,f,h,g,m,v,p;for(i=0,s=this.C;i<s;i++)c=this.belongings[i],c in t||(t[c]=r,e[r]={inAdj:{},outAdj:{},totalInWeights:this.totalInWeights[c],totalOutWeights:this.totalOutWeights[c],internalWeights:0},r++),this.belongings[i]=t[c];var b,y;if(this.keepDendrogram){for(b=this.dendrogram[this.level],y=new(it.getPointerArray(r))(n),i=0;i<n;i++)y[i]=this.belongings[b[i]];this.dendrogram.push(y)}else for(i=0;i<n;i++)this.mapping[i]=this.belongings[this.mapping[i]];for(i=0,s=this.C;i<s;i++)for(c=this.belongings[i],h=this.offsets[i],f=e[c],v=f.inAdj,p=f.outAdj,f.internalWeights+=this.loops[i],o=this.starts[i],l=this.starts[i+1];o<l;o++){if(u=this.neighborhood[o],d=this.belongings[u],g=o<h,m=g?p:v,c===d){g&&(f.internalWeights+=this.weights[o]);continue}d in m||(m[d]=0),m[d]+=this.weights[o]}for(this.C=r,u=0,c=0;c<r;c++){f=e[c],v=f.inAdj,p=f.outAdj,c=+c,this.totalInWeights[c]=f.totalInWeights,this.totalOutWeights[c]=f.totalOutWeights,this.loops[c]=f.internalWeights,this.counts[c]=1,this.starts[c]=u,this.belongings[c]=c;for(d in p)this.neighborhood[u]=+d,this.weights[u]=p[d],a++,u++;this.offsets[c]=u;for(d in v)this.neighborhood[u]=+d,this.weights[u]=v[d],a++,u++}return this.starts[r]=a,this.E=a,this.U=0,this.level++,t};le.prototype.modularity=function(){var e,t,n,r,a,i=0,o=this.M,s=new Float64Array(this.C);for(n=0;n<this.C;n++)for(e=this.belongings[n],s[e]+=this.loops[n],r=this.starts[n],a=this.offsets[n];r<a;r++)t=this.belongings[this.neighborhood[r]],e===t&&(s[e]+=this.weights[r]);for(n=0;n<this.C;n++)i+=s[n]/o-this.totalInWeights[n]*this.totalOutWeights[n]/Math.pow(o,2)*this.resolution;return i};le.prototype.delta=function(e,t,n,r,a){var i=this.M,o=this.totalInWeights[a],s=this.totalOutWeights[a],l=this.loops[e];return t+=l,n+=l,r/i-(n*o+t*s)*this.resolution/(i*i)};le.prototype.deltaWithOwnCommunity=function(e,t,n,r,a){var i=this.M,o=this.totalInWeights[a],s=this.totalOutWeights[a],l=this.loops[e];return t+=l,n+=l,r/i-(n*(o-t)+t*(s-n))*this.resolution/(i*i)};le.prototype.collect=oe.prototype.collect;le.prototype.assign=oe.prototype.assign;le.prototype[Nl]=function(){var e={};Object.defineProperty(e,"constructor",{value:le,enumerable:!1}),e.C=this.C,e.M=this.M,e.E=this.E,e.U=this.U,e.resolution=this.resolution,e.level=this.level,e.nodes=this.nodes,e.starts=this.starts.slice(0,e.C+1);var t=["neighborhood","weights"],n=["counts","offsets","loops","belongings","totalInWeights","totalOutWeights"],r=this;return t.forEach(function(a){e[a]=r[a].slice(0,e.E)}),n.forEach(function(a){e[a]=r[a].slice(0,e.C)}),e.unused=this.unused.slice(0,this.U),this.keepDendrogram?e.dendrogram=this.dendrogram:e.mapping=this.mapping,e};oi.UndirectedLouvainIndex=oe;oi.DirectedLouvainIndex=le});var jl=Ae((Sy,Hl)=>{var Xg=ri(),Yg=Bt(),Kg=xl(),Gl=Sl(),Bl=Al(),Wl=kl().createRandomIndex,Ol=Ml(),Zg=Ol.UndirectedLouvainIndex,Jg=Ol.DirectedLouvainIndex,Ul={nodeCommunityAttribute:"community",getEdgeWeight:"weight",fastLocalMoves:!0,randomWalk:!0,resolution:1,rng:Math.random};function Wr(e,t,n){var r=e.get(t);typeof r>"u"&&(r=0),r+=n,e.set(t,r)}var Qg=1e-10;function Or(e,t,n,r,a){return Math.abs(r-a)<Qg?e===t?!1:n>e:r>a}function ep(e,t,n){var r=new Zg(t,{getEdgeWeight:n.getEdgeWeight,keepDendrogram:e,resolution:n.resolution}),a=Wl(n.rng),i=!0,o=!0,s,l,u=new Gl(Float64Array,r.C),c,d,f,h,g,m,v,p,b,y,_,x,T,E,S,w,A=0,C=0,D=[],F,k;for(n.fastLocalMoves&&(c=new Bl(r.C));i;){if(y=r.C,i=!1,o=!0,n.fastLocalMoves){for(k=0,m=n.randomWalk?a(y):0,v=0;v<y;v++,m++)p=m%y,c.enqueue(p);for(;c.size!==0;){for(p=c.dequeue(),C++,_=0,u.clear(),s=r.belongings[p],d=r.starts[p],f=r.starts[p+1];d<f;d++)b=r.neighborhood[d],h=r.weights[d],l=r.belongings[b],_+=h,Wr(u,l,h);for(E=r.fastDeltaWithOwnCommunity(p,_,u.get(s)||0,s),T=s,g=0;g<u.size;g++)l=u.dense[g],l!==s&&(x=u.vals[g],A++,w=r.fastDelta(p,_,x,l),S=Or(T,s,l,w,E),S&&(E=w,T=l));if(E<0){if(T=r.isolate(p,_),T===s)continue}else{if(T===s)continue;r.move(p,_,T)}for(i=!0,k++,d=r.starts[p],f=r.starts[p+1];d<f;d++)b=r.neighborhood[d],l=r.belongings[b],l!==T&&c.enqueue(b)}D.push(k)}else for(F=[],D.push(F);o;){for(o=!1,k=0,m=n.randomWalk?a(y):0,v=0;v<y;v++,m++){for(p=m%y,C++,_=0,u.clear(),s=r.belongings[p],d=r.starts[p],f=r.starts[p+1];d<f;d++)b=r.neighborhood[d],h=r.weights[d],l=r.belongings[b],_+=h,Wr(u,l,h);for(E=r.fastDeltaWithOwnCommunity(p,_,u.get(s)||0,s),T=s,g=0;g<u.size;g++)l=u.dense[g],l!==s&&(x=u.vals[g],A++,w=r.fastDelta(p,_,x,l),S=Or(T,s,l,w,E),S&&(E=w,T=l));if(E<0){if(T=r.isolate(p,_),T===s)continue}else{if(T===s)continue;r.move(p,_,T)}o=!0,k++}F.push(k),i=o||i}i&&r.zoomOut()}var L={index:r,deltaComputations:A,nodesVisited:C,moves:D};return L}function tp(e,t,n){var r=new Jg(t,{getEdgeWeight:n.getEdgeWeight,keepDendrogram:e,resolution:n.resolution}),a=Wl(n.rng),i=!0,o=!0,s,l,u=new Gl(Float64Array,r.C),c,d,f,h,g,m,v,p,b,y,_,x,T,E,S,w,A,C,D,F=0,k=0,L=[],z,N;for(n.fastLocalMoves&&(c=new Bl(r.C));i;){if(x=r.C,i=!1,o=!0,n.fastLocalMoves){for(N=0,p=n.randomWalk?a(x):0,b=0;b<x;b++,p++)y=p%x,c.enqueue(y);for(;c.size!==0;){for(y=c.dequeue(),k++,T=0,E=0,u.clear(),s=r.belongings[y],d=r.starts[y],f=r.starts[y+1],h=r.offsets[y];d<f;d++)g=d<h,_=r.neighborhood[d],m=r.weights[d],l=r.belongings[_],g?E+=m:T+=m,Wr(u,l,m);for(A=r.deltaWithOwnCommunity(y,T,E,u.get(s)||0,s),w=s,v=0;v<u.size;v++)l=u.dense[v],l!==s&&(S=u.vals[v],F++,D=r.delta(y,T,E,S,l),C=Or(w,s,l,D,A),C&&(A=D,w=l));if(A<0){if(w=r.isolate(y,T,E),w===s)continue}else{if(w===s)continue;r.move(y,T,E,w)}for(i=!0,N++,d=r.starts[y],f=r.starts[y+1];d<f;d++)_=r.neighborhood[d],l=r.belongings[_],l!==w&&c.enqueue(_)}L.push(N)}else for(z=[],L.push(z);o;){for(o=!1,N=0,p=n.randomWalk?a(x):0,b=0;b<x;b++,p++){for(y=p%x,k++,T=0,E=0,u.clear(),s=r.belongings[y],d=r.starts[y],f=r.starts[y+1],h=r.offsets[y];d<f;d++)g=d<h,_=r.neighborhood[d],m=r.weights[d],l=r.belongings[_],g?E+=m:T+=m,Wr(u,l,m);for(A=r.deltaWithOwnCommunity(y,T,E,u.get(s)||0,s),w=s,v=0;v<u.size;v++)l=u.dense[v],l!==s&&(S=u.vals[v],F++,D=r.delta(y,T,E,S,l),C=Or(w,s,l,D,A),C&&(A=D,w=l));if(A<0){if(w=r.isolate(y,T,E),w===s)continue}else{if(w===s)continue;r.move(y,T,E,w)}o=!0,N++}z.push(N),i=o||i}i&&r.zoomOut()}var P={index:r,deltaComputations:F,nodesVisited:k,moves:L};return P}function si(e,t,n,r){if(!Yg(n))throw new Error("graphology-communities-louvain: the given graph is not a valid graphology instance.");var a=Kg(n);if(a==="mixed")throw new Error("graphology-communities-louvain: cannot run the algorithm on a true mixed graph.");r=Xg(r,Ul);var i=0;if(n.size===0){if(e){n.forEachNode(function(d){n.setNodeAttribute(d,r.nodeCommunityAttribute,i++)});return}var o={};return n.forEachNode(function(d){o[d]=i++}),t?{communities:o,count:n.order,deltaComputations:0,dendrogram:null,level:0,modularity:NaN,moves:null,nodesVisited:0,resolution:r.resolution}:o}var s=a==="undirected"?ep:tp,l=s(t,n,r),u=l.index;if(!t){if(e){u.assign(r.nodeCommunityAttribute);return}return u.collect()}var c={count:u.C,deltaComputations:l.deltaComputations,dendrogram:u.dendrogram,level:u.level,modularity:u.modularity(),moves:l.moves,nodesVisited:l.nodesVisited,resolution:r.resolution};return e?(u.assign(r.nodeCommunityAttribute),c):(c.communities=u.collect(),c)}var Ur=si.bind(null,!1,!1);Ur.assign=si.bind(null,!0,!1);Ur.detailed=si.bind(null,!1,!0);Ur.defaults=Ul;Hl.exports=Ur});var qi=Ne(dt(),1);function kc(){let e=arguments[0];for(let t=1,n=arguments.length;t<n;t++)if(arguments[t])for(let r in arguments[t])e[r]=arguments[t][r];return e}var ce=kc;typeof Object.assign=="function"&&(ce=Object.assign);function De(e,t,n,r){let a=e._nodes.get(t),i=null;return a&&(r==="mixed"?i=a.out&&a.out[n]||a.undirected&&a.undirected[n]:r==="directed"?i=a.out&&a.out[n]:i=a.undirected&&a.undirected[n]),i}function he(e){return typeof e=="object"&&e!==null}function Xi(e){let t;for(t in e)return!1;return!0}function we(e,t,n){Object.defineProperty(e,t,{enumerable:!1,configurable:!1,writable:!0,value:n})}function Fe(e,t,n){let r={enumerable:!0,configurable:!0};typeof n=="function"?r.get=n:(r.value=n,r.writable=!1),Object.defineProperty(e,t,r)}function ji(e){return!(!he(e)||e.attributes&&!Array.isArray(e.attributes))}function Lc(){let e=Math.floor(Math.random()*256)&255;return()=>e++}function qe(){let e=arguments,t=null,n=-1;return{[Symbol.iterator](){return this},next(){let r=null;do{if(t===null){if(n++,n>=e.length)return{done:!0};t=e[n][Symbol.iterator]()}if(r=t.next(),r.done){t=null;continue}break}while(!0);return r}}}function kt(){return{[Symbol.iterator](){return this},next(){return{done:!0}}}}var an=class extends Error{constructor(t){super(),this.name="GraphError",this.message=t}},O=class e extends an{constructor(t){super(t),this.name="InvalidArgumentsGraphError",typeof Error.captureStackTrace=="function"&&Error.captureStackTrace(this,e.prototype.constructor)}},W=class e extends an{constructor(t){super(t),this.name="NotFoundGraphError",typeof Error.captureStackTrace=="function"&&Error.captureStackTrace(this,e.prototype.constructor)}},V=class e extends an{constructor(t){super(t),this.name="UsageGraphError",typeof Error.captureStackTrace=="function"&&Error.captureStackTrace(this,e.prototype.constructor)}};function Yi(e,t){this.key=e,this.attributes=t,this.clear()}Yi.prototype.clear=function(){this.inDegree=0,this.outDegree=0,this.undirectedDegree=0,this.undirectedLoops=0,this.directedLoops=0,this.in={},this.out={},this.undirected={}};function Ki(e,t){this.key=e,this.attributes=t,this.clear()}Ki.prototype.clear=function(){this.inDegree=0,this.outDegree=0,this.directedLoops=0,this.in={},this.out={}};function Zi(e,t){this.key=e,this.attributes=t,this.clear()}Zi.prototype.clear=function(){this.undirectedDegree=0,this.undirectedLoops=0,this.undirected={}};function Lt(e,t,n,r,a){this.key=t,this.attributes=a,this.undirected=e,this.source=n,this.target=r}Lt.prototype.attach=function(){let e="out",t="in";this.undirected&&(e=t="undirected");let n=this.source.key,r=this.target.key;this.source[e][r]=this,!(this.undirected&&n===r)&&(this.target[t][n]=this)};Lt.prototype.attachMulti=function(){let e="out",t="in",n=this.source.key,r=this.target.key;this.undirected&&(e=t="undirected");let a=this.source[e],i=a[r];if(typeof i>"u"){a[r]=this,this.undirected&&n===r||(this.target[t][n]=this);return}i.previous=this,this.next=i,a[r]=this,this.target[t][n]=this};Lt.prototype.detach=function(){let e=this.source.key,t=this.target.key,n="out",r="in";this.undirected&&(n=r="undirected"),delete this.source[n][t],delete this.target[r][e]};Lt.prototype.detachMulti=function(){let e=this.source.key,t=this.target.key,n="out",r="in";this.undirected&&(n=r="undirected"),this.previous===void 0?this.next===void 0?(delete this.source[n][t],delete this.target[r][e]):(this.next.previous=void 0,this.source[n][t]=this.next,this.target[r][e]=this.next):(this.previous.next=this.next,this.next!==void 0&&(this.next.previous=this.previous))};var Ji=0,Qi=1,Fc=2,eo=3;function Xe(e,t,n,r,a,i,o){let s,l,u,c;if(r=""+r,n===Ji){if(s=e._nodes.get(r),!s)throw new W(`Graph.${t}: could not find the "${r}" node in the graph.`);u=a,c=i}else if(n===eo){if(a=""+a,l=e._edges.get(a),!l)throw new W(`Graph.${t}: could not find the "${a}" edge in the graph.`);let d=l.source.key,f=l.target.key;if(r===d)s=l.target;else if(r===f)s=l.source;else throw new W(`Graph.${t}: the "${r}" node is not attached to the "${a}" edge (${d}, ${f}).`);u=i,c=o}else{if(l=e._edges.get(r),!l)throw new W(`Graph.${t}: could not find the "${r}" edge in the graph.`);n===Qi?s=l.source:s=l.target,u=a,c=i}return[s,u,c]}function Pc(e,t,n){e.prototype[t]=function(r,a,i){let[o,s]=Xe(this,t,n,r,a,i);return o.attributes[s]}}function Ic(e,t,n){e.prototype[t]=function(r,a){let[i]=Xe(this,t,n,r,a);return i.attributes}}function Nc(e,t,n){e.prototype[t]=function(r,a,i){let[o,s]=Xe(this,t,n,r,a,i);return o.attributes.hasOwnProperty(s)}}function zc(e,t,n){e.prototype[t]=function(r,a,i,o){let[s,l,u]=Xe(this,t,n,r,a,i,o);return s.attributes[l]=u,this.emit("nodeAttributesUpdated",{key:s.key,type:"set",attributes:s.attributes,name:l}),this}}function Mc(e,t,n){e.prototype[t]=function(r,a,i,o){let[s,l,u]=Xe(this,t,n,r,a,i,o);if(typeof u!="function")throw new O(`Graph.${t}: updater should be a function.`);let c=s.attributes,d=u(c[l]);return c[l]=d,this.emit("nodeAttributesUpdated",{key:s.key,type:"set",attributes:s.attributes,name:l}),this}}function Gc(e,t,n){e.prototype[t]=function(r,a,i){let[o,s]=Xe(this,t,n,r,a,i);return delete o.attributes[s],this.emit("nodeAttributesUpdated",{key:o.key,type:"remove",attributes:o.attributes,name:s}),this}}function Bc(e,t,n){e.prototype[t]=function(r,a,i){let[o,s]=Xe(this,t,n,r,a,i);if(!he(s))throw new O(`Graph.${t}: provided attributes are not a plain object.`);return o.attributes=s,this.emit("nodeAttributesUpdated",{key:o.key,type:"replace",attributes:o.attributes}),this}}function Wc(e,t,n){e.prototype[t]=function(r,a,i){let[o,s]=Xe(this,t,n,r,a,i);if(!he(s))throw new O(`Graph.${t}: provided attributes are not a plain object.`);return ce(o.attributes,s),this.emit("nodeAttributesUpdated",{key:o.key,type:"merge",attributes:o.attributes,data:s}),this}}function Oc(e,t,n){e.prototype[t]=function(r,a,i){let[o,s]=Xe(this,t,n,r,a,i);if(typeof s!="function")throw new O(`Graph.${t}: provided updater is not a function.`);return o.attributes=s(o.attributes),this.emit("nodeAttributesUpdated",{key:o.key,type:"update",attributes:o.attributes}),this}}var Uc=[{name:e=>`get${e}Attribute`,attacher:Pc},{name:e=>`get${e}Attributes`,attacher:Ic},{name:e=>`has${e}Attribute`,attacher:Nc},{name:e=>`set${e}Attribute`,attacher:zc},{name:e=>`update${e}Attribute`,attacher:Mc},{name:e=>`remove${e}Attribute`,attacher:Gc},{name:e=>`replace${e}Attributes`,attacher:Bc},{name:e=>`merge${e}Attributes`,attacher:Wc},{name:e=>`update${e}Attributes`,attacher:Oc}];function Hc(e){Uc.forEach(function({name:t,attacher:n}){n(e,t("Node"),Ji),n(e,t("Source"),Qi),n(e,t("Target"),Fc),n(e,t("Opposite"),eo)})}function jc(e,t,n){e.prototype[t]=function(r,a){let i;if(this.type!=="mixed"&&n!=="mixed"&&n!==this.type)throw new V(`Graph.${t}: cannot find this type of edges in your ${this.type} graph.`);if(arguments.length>2){if(this.multi)throw new V(`Graph.${t}: cannot use a {source,target} combo when asking about an edge's attributes in a MultiGraph since we cannot infer the one you want information about.`);let o=""+r,s=""+a;if(a=arguments[2],i=De(this,o,s,n),!i)throw new W(`Graph.${t}: could not find an edge for the given path ("${o}" - "${s}").`)}else{if(n!=="mixed")throw new V(`Graph.${t}: calling this method with only a key (vs. a source and target) does not make sense since an edge with this key could have the other type.`);if(r=""+r,i=this._edges.get(r),!i)throw new W(`Graph.${t}: could not find the "${r}" edge in the graph.`)}return i.attributes[a]}}function $c(e,t,n){e.prototype[t]=function(r){let a;if(this.type!=="mixed"&&n!=="mixed"&&n!==this.type)throw new V(`Graph.${t}: cannot find this type of edges in your ${this.type} graph.`);if(arguments.length>1){if(this.multi)throw new V(`Graph.${t}: cannot use a {source,target} combo when asking about an edge's attributes in a MultiGraph since we cannot infer the one you want information about.`);let i=""+r,o=""+arguments[1];if(a=De(this,i,o,n),!a)throw new W(`Graph.${t}: could not find an edge for the given path ("${i}" - "${o}").`)}else{if(n!=="mixed")throw new V(`Graph.${t}: calling this method with only a key (vs. a source and target) does not make sense since an edge with this key could have the other type.`);if(r=""+r,a=this._edges.get(r),!a)throw new W(`Graph.${t}: could not find the "${r}" edge in the graph.`)}return a.attributes}}function Vc(e,t,n){e.prototype[t]=function(r,a){let i;if(this.type!=="mixed"&&n!=="mixed"&&n!==this.type)throw new V(`Graph.${t}: cannot find this type of edges in your ${this.type} graph.`);if(arguments.length>2){if(this.multi)throw new V(`Graph.${t}: cannot use a {source,target} combo when asking about an edge's attributes in a MultiGraph since we cannot infer the one you want information about.`);let o=""+r,s=""+a;if(a=arguments[2],i=De(this,o,s,n),!i)throw new W(`Graph.${t}: could not find an edge for the given path ("${o}" - "${s}").`)}else{if(n!=="mixed")throw new V(`Graph.${t}: calling this method with only a key (vs. a source and target) does not make sense since an edge with this key could have the other type.`);if(r=""+r,i=this._edges.get(r),!i)throw new W(`Graph.${t}: could not find the "${r}" edge in the graph.`)}return i.attributes.hasOwnProperty(a)}}function qc(e,t,n){e.prototype[t]=function(r,a,i){let o;if(this.type!=="mixed"&&n!=="mixed"&&n!==this.type)throw new V(`Graph.${t}: cannot find this type of edges in your ${this.type} graph.`);if(arguments.length>3){if(this.multi)throw new V(`Graph.${t}: cannot use a {source,target} combo when asking about an edge's attributes in a MultiGraph since we cannot infer the one you want information about.`);let s=""+r,l=""+a;if(a=arguments[2],i=arguments[3],o=De(this,s,l,n),!o)throw new W(`Graph.${t}: could not find an edge for the given path ("${s}" - "${l}").`)}else{if(n!=="mixed")throw new V(`Graph.${t}: calling this method with only a key (vs. a source and target) does not make sense since an edge with this key could have the other type.`);if(r=""+r,o=this._edges.get(r),!o)throw new W(`Graph.${t}: could not find the "${r}" edge in the graph.`)}return o.attributes[a]=i,this.emit("edgeAttributesUpdated",{key:o.key,type:"set",attributes:o.attributes,name:a}),this}}function Xc(e,t,n){e.prototype[t]=function(r,a,i){let o;if(this.type!=="mixed"&&n!=="mixed"&&n!==this.type)throw new V(`Graph.${t}: cannot find this type of edges in your ${this.type} graph.`);if(arguments.length>3){if(this.multi)throw new V(`Graph.${t}: cannot use a {source,target} combo when asking about an edge's attributes in a MultiGraph since we cannot infer the one you want information about.`);let s=""+r,l=""+a;if(a=arguments[2],i=arguments[3],o=De(this,s,l,n),!o)throw new W(`Graph.${t}: could not find an edge for the given path ("${s}" - "${l}").`)}else{if(n!=="mixed")throw new V(`Graph.${t}: calling this method with only a key (vs. a source and target) does not make sense since an edge with this key could have the other type.`);if(r=""+r,o=this._edges.get(r),!o)throw new W(`Graph.${t}: could not find the "${r}" edge in the graph.`)}if(typeof i!="function")throw new O(`Graph.${t}: updater should be a function.`);return o.attributes[a]=i(o.attributes[a]),this.emit("edgeAttributesUpdated",{key:o.key,type:"set",attributes:o.attributes,name:a}),this}}function Yc(e,t,n){e.prototype[t]=function(r,a){let i;if(this.type!=="mixed"&&n!=="mixed"&&n!==this.type)throw new V(`Graph.${t}: cannot find this type of edges in your ${this.type} graph.`);if(arguments.length>2){if(this.multi)throw new V(`Graph.${t}: cannot use a {source,target} combo when asking about an edge's attributes in a MultiGraph since we cannot infer the one you want information about.`);let o=""+r,s=""+a;if(a=arguments[2],i=De(this,o,s,n),!i)throw new W(`Graph.${t}: could not find an edge for the given path ("${o}" - "${s}").`)}else{if(n!=="mixed")throw new V(`Graph.${t}: calling this method with only a key (vs. a source and target) does not make sense since an edge with this key could have the other type.`);if(r=""+r,i=this._edges.get(r),!i)throw new W(`Graph.${t}: could not find the "${r}" edge in the graph.`)}return delete i.attributes[a],this.emit("edgeAttributesUpdated",{key:i.key,type:"remove",attributes:i.attributes,name:a}),this}}function Kc(e,t,n){e.prototype[t]=function(r,a){let i;if(this.type!=="mixed"&&n!=="mixed"&&n!==this.type)throw new V(`Graph.${t}: cannot find this type of edges in your ${this.type} graph.`);if(arguments.length>2){if(this.multi)throw new V(`Graph.${t}: cannot use a {source,target} combo when asking about an edge's attributes in a MultiGraph since we cannot infer the one you want information about.`);let o=""+r,s=""+a;if(a=arguments[2],i=De(this,o,s,n),!i)throw new W(`Graph.${t}: could not find an edge for the given path ("${o}" - "${s}").`)}else{if(n!=="mixed")throw new V(`Graph.${t}: calling this method with only a key (vs. a source and target) does not make sense since an edge with this key could have the other type.`);if(r=""+r,i=this._edges.get(r),!i)throw new W(`Graph.${t}: could not find the "${r}" edge in the graph.`)}if(!he(a))throw new O(`Graph.${t}: provided attributes are not a plain object.`);return i.attributes=a,this.emit("edgeAttributesUpdated",{key:i.key,type:"replace",attributes:i.attributes}),this}}function Zc(e,t,n){e.prototype[t]=function(r,a){let i;if(this.type!=="mixed"&&n!=="mixed"&&n!==this.type)throw new V(`Graph.${t}: cannot find this type of edges in your ${this.type} graph.`);if(arguments.length>2){if(this.multi)throw new V(`Graph.${t}: cannot use a {source,target} combo when asking about an edge's attributes in a MultiGraph since we cannot infer the one you want information about.`);let o=""+r,s=""+a;if(a=arguments[2],i=De(this,o,s,n),!i)throw new W(`Graph.${t}: could not find an edge for the given path ("${o}" - "${s}").`)}else{if(n!=="mixed")throw new V(`Graph.${t}: calling this method with only a key (vs. a source and target) does not make sense since an edge with this key could have the other type.`);if(r=""+r,i=this._edges.get(r),!i)throw new W(`Graph.${t}: could not find the "${r}" edge in the graph.`)}if(!he(a))throw new O(`Graph.${t}: provided attributes are not a plain object.`);return ce(i.attributes,a),this.emit("edgeAttributesUpdated",{key:i.key,type:"merge",attributes:i.attributes,data:a}),this}}function Jc(e,t,n){e.prototype[t]=function(r,a){let i;if(this.type!=="mixed"&&n!=="mixed"&&n!==this.type)throw new V(`Graph.${t}: cannot find this type of edges in your ${this.type} graph.`);if(arguments.length>2){if(this.multi)throw new V(`Graph.${t}: cannot use a {source,target} combo when asking about an edge's attributes in a MultiGraph since we cannot infer the one you want information about.`);let o=""+r,s=""+a;if(a=arguments[2],i=De(this,o,s,n),!i)throw new W(`Graph.${t}: could not find an edge for the given path ("${o}" - "${s}").`)}else{if(n!=="mixed")throw new V(`Graph.${t}: calling this method with only a key (vs. a source and target) does not make sense since an edge with this key could have the other type.`);if(r=""+r,i=this._edges.get(r),!i)throw new W(`Graph.${t}: could not find the "${r}" edge in the graph.`)}if(typeof a!="function")throw new O(`Graph.${t}: provided updater is not a function.`);return i.attributes=a(i.attributes),this.emit("edgeAttributesUpdated",{key:i.key,type:"update",attributes:i.attributes}),this}}var Qc=[{name:e=>`get${e}Attribute`,attacher:jc},{name:e=>`get${e}Attributes`,attacher:$c},{name:e=>`has${e}Attribute`,attacher:Vc},{name:e=>`set${e}Attribute`,attacher:qc},{name:e=>`update${e}Attribute`,attacher:Xc},{name:e=>`remove${e}Attribute`,attacher:Yc},{name:e=>`replace${e}Attributes`,attacher:Kc},{name:e=>`merge${e}Attributes`,attacher:Zc},{name:e=>`update${e}Attributes`,attacher:Jc}];function ed(e){Qc.forEach(function({name:t,attacher:n}){n(e,t("Edge"),"mixed"),n(e,t("DirectedEdge"),"directed"),n(e,t("UndirectedEdge"),"undirected")})}var td=[{name:"edges",type:"mixed"},{name:"inEdges",type:"directed",direction:"in"},{name:"outEdges",type:"directed",direction:"out"},{name:"inboundEdges",type:"mixed",direction:"in"},{name:"outboundEdges",type:"mixed",direction:"out"},{name:"directedEdges",type:"directed"},{name:"undirectedEdges",type:"undirected"}];function nd(e,t,n,r){let a=!1;for(let i in t){if(i===r)continue;let o=t[i];if(a=n(o.key,o.attributes,o.source.key,o.target.key,o.source.attributes,o.target.attributes,o.undirected),e&&a)return o.key}}function rd(e,t,n,r){let a,i,o,s=!1;for(let l in t)if(l!==r){a=t[l];do{if(i=a.source,o=a.target,s=n(a.key,a.attributes,i.key,o.key,i.attributes,o.attributes,a.undirected),e&&s)return a.key;a=a.next}while(a!==void 0)}}function ia(e,t){let n=Object.keys(e),r=n.length,a,i=0;return{[Symbol.iterator](){return this},next(){do if(a)a=a.next;else{if(i>=r)return{done:!0};let o=n[i++];if(o===t){a=void 0;continue}a=e[o]}while(!a);return{done:!1,value:{edge:a.key,attributes:a.attributes,source:a.source.key,target:a.target.key,sourceAttributes:a.source.attributes,targetAttributes:a.target.attributes,undirected:a.undirected}}}}}function ad(e,t,n,r){let a=t[n];if(!a)return;let i=a.source,o=a.target;if(r(a.key,a.attributes,i.key,o.key,i.attributes,o.attributes,a.undirected)&&e)return a.key}function id(e,t,n,r){let a=t[n];if(!a)return;let i=!1;do{if(i=r(a.key,a.attributes,a.source.key,a.target.key,a.source.attributes,a.target.attributes,a.undirected),e&&i)return a.key;a=a.next}while(a!==void 0)}function oa(e,t){let n=e[t];if(n.next!==void 0)return{[Symbol.iterator](){return this},next(){if(!n)return{done:!0};let a={edge:n.key,attributes:n.attributes,source:n.source.key,target:n.target.key,sourceAttributes:n.source.attributes,targetAttributes:n.target.attributes,undirected:n.undirected};return n=n.next,{done:!1,value:a}}};let r=!1;return{[Symbol.iterator](){return this},next(){return r===!0?{done:!0}:(r=!0,{done:!1,value:{edge:n.key,attributes:n.attributes,source:n.source.key,target:n.target.key,sourceAttributes:n.source.attributes,targetAttributes:n.target.attributes,undirected:n.undirected}})}}}function od(e,t){if(e.size===0)return[];if(t==="mixed"||t===e.type)return Array.from(e._edges.keys());let n=t==="undirected"?e.undirectedSize:e.directedSize,r=new Array(n),a=t==="undirected",i=e._edges.values(),o=0,s,l;for(;s=i.next(),s.done!==!0;)l=s.value,l.undirected===a&&(r[o++]=l.key);return r}function to(e,t,n,r){if(t.size===0)return;let a=n!=="mixed"&&n!==t.type,i=n==="undirected",o,s,l=!1,u=t._edges.values();for(;o=u.next(),o.done!==!0;){if(s=o.value,a&&s.undirected!==i)continue;let{key:c,attributes:d,source:f,target:h}=s;if(l=r(c,d,f.key,h.key,f.attributes,h.attributes,s.undirected),e&&l)return c}}function sd(e,t){if(e.size===0)return kt();let n=t!=="mixed"&&t!==e.type,r=t==="undirected",a=e._edges.values();return{[Symbol.iterator](){return this},next(){let i,o;for(;;){if(i=a.next(),i.done)return i;if(o=i.value,!(n&&o.undirected!==r))break}return{value:{edge:o.key,attributes:o.attributes,source:o.source.key,target:o.target.key,sourceAttributes:o.source.attributes,targetAttributes:o.target.attributes,undirected:o.undirected},done:!1}}}}function sa(e,t,n,r,a,i){let o=t?rd:nd,s;if(n!=="undirected"&&(r!=="out"&&(s=o(e,a.in,i),e&&s)||r!=="in"&&(s=o(e,a.out,i,r?void 0:a.key),e&&s))||n!=="directed"&&(s=o(e,a.undirected,i),e&&s))return s}function ld(e,t,n,r){let a=[];return sa(!1,e,t,n,r,function(i){a.push(i)}),a}function ud(e,t,n){let r=kt();return e!=="undirected"&&(t!=="out"&&typeof n.in<"u"&&(r=qe(r,ia(n.in))),t!=="in"&&typeof n.out<"u"&&(r=qe(r,ia(n.out,t?void 0:n.key)))),e!=="directed"&&typeof n.undirected<"u"&&(r=qe(r,ia(n.undirected))),r}function la(e,t,n,r,a,i,o){let s=n?id:ad,l;if(t!=="undirected"&&(typeof a.in<"u"&&r!=="out"&&(l=s(e,a.in,i,o),e&&l)||typeof a.out<"u"&&r!=="in"&&(r||a.key!==i)&&(l=s(e,a.out,i,o),e&&l))||t!=="directed"&&typeof a.undirected<"u"&&(l=s(e,a.undirected,i,o),e&&l))return l}function cd(e,t,n,r,a){let i=[];return la(!1,e,t,n,r,a,function(o){i.push(o)}),i}function dd(e,t,n,r){let a=kt();return e!=="undirected"&&(typeof n.in<"u"&&t!=="out"&&r in n.in&&(a=qe(a,oa(n.in,r))),typeof n.out<"u"&&t!=="in"&&r in n.out&&(t||n.key!==r)&&(a=qe(a,oa(n.out,r)))),e!=="directed"&&typeof n.undirected<"u"&&r in n.undirected&&(a=qe(a,oa(n.undirected,r))),a}function hd(e,t){let{name:n,type:r,direction:a}=t;e.prototype[n]=function(i,o){if(r!=="mixed"&&this.type!=="mixed"&&r!==this.type)return[];if(!arguments.length)return od(this,r);if(arguments.length===1){i=""+i;let s=this._nodes.get(i);if(typeof s>"u")throw new W(`Graph.${n}: could not find the "${i}" node in the graph.`);return ld(this.multi,r==="mixed"?this.type:r,a,s)}if(arguments.length===2){i=""+i,o=""+o;let s=this._nodes.get(i);if(!s)throw new W(`Graph.${n}:  could not find the "${i}" source node in the graph.`);if(!this._nodes.has(o))throw new W(`Graph.${n}:  could not find the "${o}" target node in the graph.`);return cd(r,this.multi,a,s,o)}throw new O(`Graph.${n}: too many arguments (expecting 0, 1 or 2 and got ${arguments.length}).`)}}function fd(e,t){let{name:n,type:r,direction:a}=t,i="forEach"+n[0].toUpperCase()+n.slice(1,-1);e.prototype[i]=function(u,c,d){if(!(r!=="mixed"&&this.type!=="mixed"&&r!==this.type)){if(arguments.length===1)return d=u,to(!1,this,r,d);if(arguments.length===2){u=""+u,d=c;let f=this._nodes.get(u);if(typeof f>"u")throw new W(`Graph.${i}: could not find the "${u}" node in the graph.`);return sa(!1,this.multi,r==="mixed"?this.type:r,a,f,d)}if(arguments.length===3){u=""+u,c=""+c;let f=this._nodes.get(u);if(!f)throw new W(`Graph.${i}:  could not find the "${u}" source node in the graph.`);if(!this._nodes.has(c))throw new W(`Graph.${i}:  could not find the "${c}" target node in the graph.`);return la(!1,r,this.multi,a,f,c,d)}throw new O(`Graph.${i}: too many arguments (expecting 1, 2 or 3 and got ${arguments.length}).`)}};let o="map"+n[0].toUpperCase()+n.slice(1);e.prototype[o]=function(){let u=Array.prototype.slice.call(arguments),c=u.pop(),d;if(u.length===0){let f=0;r!=="directed"&&(f+=this.undirectedSize),r!=="undirected"&&(f+=this.directedSize),d=new Array(f);let h=0;u.push((g,m,v,p,b,y,_)=>{d[h++]=c(g,m,v,p,b,y,_)})}else d=[],u.push((f,h,g,m,v,p,b)=>{d.push(c(f,h,g,m,v,p,b))});return this[i].apply(this,u),d};let s="filter"+n[0].toUpperCase()+n.slice(1);e.prototype[s]=function(){let u=Array.prototype.slice.call(arguments),c=u.pop(),d=[];return u.push((f,h,g,m,v,p,b)=>{c(f,h,g,m,v,p,b)&&d.push(f)}),this[i].apply(this,u),d};let l="reduce"+n[0].toUpperCase()+n.slice(1);e.prototype[l]=function(){let u=Array.prototype.slice.call(arguments);if(u.length<2||u.length>4)throw new O(`Graph.${l}: invalid number of arguments (expecting 2, 3 or 4 and got ${u.length}).`);if(typeof u[u.length-1]=="function"&&typeof u[u.length-2]!="function")throw new O(`Graph.${l}: missing initial value. You must provide it because the callback takes more than one argument and we cannot infer the initial value from the first iteration, as you could with a simple array.`);let c,d;u.length===2?(c=u[0],d=u[1],u=[]):u.length===3?(c=u[1],d=u[2],u=[u[0]]):u.length===4&&(c=u[2],d=u[3],u=[u[0],u[1]]);let f=d;return u.push((h,g,m,v,p,b,y)=>{f=c(f,h,g,m,v,p,b,y)}),this[i].apply(this,u),f}}function gd(e,t){let{name:n,type:r,direction:a}=t,i="find"+n[0].toUpperCase()+n.slice(1,-1);e.prototype[i]=function(l,u,c){if(r!=="mixed"&&this.type!=="mixed"&&r!==this.type)return!1;if(arguments.length===1)return c=l,to(!0,this,r,c);if(arguments.length===2){l=""+l,c=u;let d=this._nodes.get(l);if(typeof d>"u")throw new W(`Graph.${i}: could not find the "${l}" node in the graph.`);return sa(!0,this.multi,r==="mixed"?this.type:r,a,d,c)}if(arguments.length===3){l=""+l,u=""+u;let d=this._nodes.get(l);if(!d)throw new W(`Graph.${i}:  could not find the "${l}" source node in the graph.`);if(!this._nodes.has(u))throw new W(`Graph.${i}:  could not find the "${u}" target node in the graph.`);return la(!0,r,this.multi,a,d,u,c)}throw new O(`Graph.${i}: too many arguments (expecting 1, 2 or 3 and got ${arguments.length}).`)};let o="some"+n[0].toUpperCase()+n.slice(1,-1);e.prototype[o]=function(){let l=Array.prototype.slice.call(arguments),u=l.pop();return l.push((d,f,h,g,m,v,p)=>u(d,f,h,g,m,v,p)),!!this[i].apply(this,l)};let s="every"+n[0].toUpperCase()+n.slice(1,-1);e.prototype[s]=function(){let l=Array.prototype.slice.call(arguments),u=l.pop();return l.push((d,f,h,g,m,v,p)=>!u(d,f,h,g,m,v,p)),!this[i].apply(this,l)}}function pd(e,t){let{name:n,type:r,direction:a}=t,i=n.slice(0,-1)+"Entries";e.prototype[i]=function(o,s){if(r!=="mixed"&&this.type!=="mixed"&&r!==this.type)return kt();if(!arguments.length)return sd(this,r);if(arguments.length===1){o=""+o;let l=this._nodes.get(o);if(!l)throw new W(`Graph.${i}: could not find the "${o}" node in the graph.`);return ud(r,a,l)}if(arguments.length===2){o=""+o,s=""+s;let l=this._nodes.get(o);if(!l)throw new W(`Graph.${i}:  could not find the "${o}" source node in the graph.`);if(!this._nodes.has(s))throw new W(`Graph.${i}:  could not find the "${s}" target node in the graph.`);return dd(r,a,l,s)}throw new O(`Graph.${i}: too many arguments (expecting 0, 1 or 2 and got ${arguments.length}).`)}}function vd(e){td.forEach(t=>{hd(e,t),fd(e,t),gd(e,t),pd(e,t)})}var md=[{name:"neighbors",type:"mixed"},{name:"inNeighbors",type:"directed",direction:"in"},{name:"outNeighbors",type:"directed",direction:"out"},{name:"inboundNeighbors",type:"mixed",direction:"in"},{name:"outboundNeighbors",type:"mixed",direction:"out"},{name:"directedNeighbors",type:"directed"},{name:"undirectedNeighbors",type:"undirected"}];function Kn(){this.A=null,this.B=null}Kn.prototype.wrap=function(e){this.A===null?this.A=e:this.B===null&&(this.B=e)};Kn.prototype.has=function(e){return this.A!==null&&e in this.A||this.B!==null&&e in this.B};function nn(e,t,n,r,a){for(let i in r){let o=r[i],s=o.source,l=o.target,u=s===n?l:s;if(t&&t.has(u.key))continue;let c=a(u.key,u.attributes);if(e&&c)return u.key}}function ua(e,t,n,r,a){if(t!=="mixed"){if(t==="undirected")return nn(e,null,r,r.undirected,a);if(typeof n=="string")return nn(e,null,r,r[n],a)}let i=new Kn,o;if(t!=="undirected"){if(n!=="out"){if(o=nn(e,null,r,r.in,a),e&&o)return o;i.wrap(r.in)}if(n!=="in"){if(o=nn(e,i,r,r.out,a),e&&o)return o;i.wrap(r.out)}}if(t!=="directed"&&(o=nn(e,i,r,r.undirected,a),e&&o))return o}function bd(e,t,n){if(e!=="mixed"){if(e==="undirected")return Object.keys(n.undirected);if(typeof t=="string")return Object.keys(n[t])}let r=[];return ua(!1,e,t,n,function(a){r.push(a)}),r}function rn(e,t,n){let r=Object.keys(n),a=r.length,i=0;return{[Symbol.iterator](){return this},next(){let o=null;do{if(i>=a)return e&&e.wrap(n),{done:!0};let s=n[r[i++]],l=s.source,u=s.target;if(o=l===t?u:l,e&&e.has(o.key)){o=null;continue}}while(o===null);return{done:!1,value:{neighbor:o.key,attributes:o.attributes}}}}}function yd(e,t,n){if(e!=="mixed"){if(e==="undirected")return rn(null,n,n.undirected);if(typeof t=="string")return rn(null,n,n[t])}let r=kt(),a=new Kn;return e!=="undirected"&&(t!=="out"&&(r=qe(r,rn(a,n,n.in))),t!=="in"&&(r=qe(r,rn(a,n,n.out)))),e!=="directed"&&(r=qe(r,rn(a,n,n.undirected))),r}function _d(e,t){let{name:n,type:r,direction:a}=t;e.prototype[n]=function(i){if(r!=="mixed"&&this.type!=="mixed"&&r!==this.type)return[];i=""+i;let o=this._nodes.get(i);if(typeof o>"u")throw new W(`Graph.${n}: could not find the "${i}" node in the graph.`);return bd(r==="mixed"?this.type:r,a,o)}}function xd(e,t){let{name:n,type:r,direction:a}=t,i="forEach"+n[0].toUpperCase()+n.slice(1,-1);e.prototype[i]=function(u,c){if(r!=="mixed"&&this.type!=="mixed"&&r!==this.type)return;u=""+u;let d=this._nodes.get(u);if(typeof d>"u")throw new W(`Graph.${i}: could not find the "${u}" node in the graph.`);ua(!1,r==="mixed"?this.type:r,a,d,c)};let o="map"+n[0].toUpperCase()+n.slice(1);e.prototype[o]=function(u,c){let d=[];return this[i](u,(f,h)=>{d.push(c(f,h))}),d};let s="filter"+n[0].toUpperCase()+n.slice(1);e.prototype[s]=function(u,c){let d=[];return this[i](u,(f,h)=>{c(f,h)&&d.push(f)}),d};let l="reduce"+n[0].toUpperCase()+n.slice(1);e.prototype[l]=function(u,c,d){if(arguments.length<3)throw new O(`Graph.${l}: missing initial value. You must provide it because the callback takes more than one argument and we cannot infer the initial value from the first iteration, as you could with a simple array.`);let f=d;return this[i](u,(h,g)=>{f=c(f,h,g)}),f}}function Td(e,t){let{name:n,type:r,direction:a}=t,i=n[0].toUpperCase()+n.slice(1,-1),o="find"+i;e.prototype[o]=function(u,c){if(r!=="mixed"&&this.type!=="mixed"&&r!==this.type)return;u=""+u;let d=this._nodes.get(u);if(typeof d>"u")throw new W(`Graph.${o}: could not find the "${u}" node in the graph.`);return ua(!0,r==="mixed"?this.type:r,a,d,c)};let s="some"+i;e.prototype[s]=function(u,c){return!!this[o](u,c)};let l="every"+i;e.prototype[l]=function(u,c){return!this[o](u,(f,h)=>!c(f,h))}}function Ed(e,t){let{name:n,type:r,direction:a}=t,i=n.slice(0,-1)+"Entries";e.prototype[i]=function(o){if(r!=="mixed"&&this.type!=="mixed"&&r!==this.type)return kt();o=""+o;let s=this._nodes.get(o);if(typeof s>"u")throw new W(`Graph.${i}: could not find the "${o}" node in the graph.`);return yd(r==="mixed"?this.type:r,a,s)}}function Sd(e){md.forEach(t=>{_d(e,t),xd(e,t),Td(e,t),Ed(e,t)})}function jn(e,t,n,r,a){let i=r._nodes.values(),o=r.type,s,l,u,c,d,f,h;for(;s=i.next(),s.done!==!0;){let g=!1;if(l=s.value,o!=="undirected"){c=l.out;for(u in c){d=c[u];do{if(f=d.target,g=!0,h=a(l.key,f.key,l.attributes,f.attributes,d.key,d.attributes,d.undirected),e&&h)return d;d=d.next}while(d)}}if(o!=="directed"){c=l.undirected;for(u in c)if(!(t&&l.key>u)){d=c[u];do{if(f=d.target,f.key!==u&&(f=d.source),g=!0,h=a(l.key,f.key,l.attributes,f.attributes,d.key,d.attributes,d.undirected),e&&h)return d;d=d.next}while(d)}}if(n&&!g&&(h=a(l.key,null,l.attributes,null,null,null,null),e&&h))return null}}function Rd(e,t){let n={key:e};return Xi(t.attributes)||(n.attributes=ce({},t.attributes)),n}function Ad(e,t,n){let r={key:t,source:n.source.key,target:n.target.key};return Xi(n.attributes)||(r.attributes=ce({},n.attributes)),e==="mixed"&&n.undirected&&(r.undirected=!0),r}function wd(e){if(!he(e))throw new O('Graph.import: invalid serialized node. A serialized node should be a plain object with at least a "key" property.');if(!("key"in e))throw new O("Graph.import: serialized node is missing its key.");if("attributes"in e&&(!he(e.attributes)||e.attributes===null))throw new O("Graph.import: invalid attributes. Attributes should be a plain object, null or omitted.")}function Dd(e){if(!he(e))throw new O('Graph.import: invalid serialized edge. A serialized edge should be a plain object with at least a "source" & "target" property.');if(!("source"in e))throw new O("Graph.import: serialized edge is missing its source.");if(!("target"in e))throw new O("Graph.import: serialized edge is missing its target.");if("attributes"in e&&(!he(e.attributes)||e.attributes===null))throw new O("Graph.import: invalid attributes. Attributes should be a plain object, null or omitted.");if("undirected"in e&&typeof e.undirected!="boolean")throw new O("Graph.import: invalid undirectedness information. Undirected should be boolean or omitted.")}var Cd=Lc(),kd=new Set(["directed","undirected","mixed"]),$i=new Set(["domain","_events","_eventsCount","_maxListeners"]),Ld=[{name:e=>`${e}Edge`,generateKey:!0},{name:e=>`${e}DirectedEdge`,generateKey:!0,type:"directed"},{name:e=>`${e}UndirectedEdge`,generateKey:!0,type:"undirected"},{name:e=>`${e}EdgeWithKey`},{name:e=>`${e}DirectedEdgeWithKey`,type:"directed"},{name:e=>`${e}UndirectedEdgeWithKey`,type:"undirected"}],Fd={allowSelfLoops:!0,multi:!1,type:"mixed"};function Pd(e,t,n){if(n&&!he(n))throw new O(`Graph.addNode: invalid attributes. Expecting an object but got "${n}"`);if(t=""+t,n=n||{},e._nodes.has(t))throw new V(`Graph.addNode: the "${t}" node already exist in the graph.`);let r=new e.NodeDataClass(t,n);return e._nodes.set(t,r),e.emit("nodeAdded",{key:t,attributes:n}),r}function Vi(e,t,n){let r=new e.NodeDataClass(t,n);return e._nodes.set(t,r),e.emit("nodeAdded",{key:t,attributes:n}),r}function no(e,t,n,r,a,i,o,s){if(!r&&e.type==="undirected")throw new V(`Graph.${t}: you cannot add a directed edge to an undirected graph. Use the #.addEdge or #.addUndirectedEdge instead.`);if(r&&e.type==="directed")throw new V(`Graph.${t}: you cannot add an undirected edge to a directed graph. Use the #.addEdge or #.addDirectedEdge instead.`);if(s&&!he(s))throw new O(`Graph.${t}: invalid attributes. Expecting an object but got "${s}"`);if(i=""+i,o=""+o,s=s||{},!e.allowSelfLoops&&i===o)throw new V(`Graph.${t}: source & target are the same ("${i}"), thus creating a loop explicitly forbidden by this graph 'allowSelfLoops' option set to false.`);let l=e._nodes.get(i),u=e._nodes.get(o);if(!l)throw new W(`Graph.${t}: source node "${i}" not found.`);if(!u)throw new W(`Graph.${t}: target node "${o}" not found.`);let c={key:null,undirected:r,source:i,target:o,attributes:s};if(n)a=e._edgeKeyGenerator();else if(a=""+a,e._edges.has(a))throw new V(`Graph.${t}: the "${a}" edge already exists in the graph.`);if(!e.multi&&(r?typeof l.undirected[o]<"u":typeof l.out[o]<"u"))throw new V(`Graph.${t}: an edge linking "${i}" to "${o}" already exists. If you really want to add multiple edges linking those nodes, you should create a multi graph by using the 'multi' option.`);let d=new Lt(r,a,l,u,s);e._edges.set(a,d);let f=i===o;return r?(l.undirectedDegree++,u.undirectedDegree++,f&&(l.undirectedLoops++,e._undirectedSelfLoopCount++)):(l.outDegree++,u.inDegree++,f&&(l.directedLoops++,e._directedSelfLoopCount++)),e.multi?d.attachMulti():d.attach(),r?e._undirectedSize++:e._directedSize++,c.key=a,e.emit("edgeAdded",c),a}function Id(e,t,n,r,a,i,o,s,l){if(!r&&e.type==="undirected")throw new V(`Graph.${t}: you cannot merge/update a directed edge to an undirected graph. Use the #.mergeEdge/#.updateEdge or #.addUndirectedEdge instead.`);if(r&&e.type==="directed")throw new V(`Graph.${t}: you cannot merge/update an undirected edge to a directed graph. Use the #.mergeEdge/#.updateEdge or #.addDirectedEdge instead.`);if(s){if(l){if(typeof s!="function")throw new O(`Graph.${t}: invalid updater function. Expecting a function but got "${s}"`)}else if(!he(s))throw new O(`Graph.${t}: invalid attributes. Expecting an object but got "${s}"`)}i=""+i,o=""+o;let u;if(l&&(u=s,s=void 0),!e.allowSelfLoops&&i===o)throw new V(`Graph.${t}: source & target are the same ("${i}"), thus creating a loop explicitly forbidden by this graph 'allowSelfLoops' option set to false.`);let c=e._nodes.get(i),d=e._nodes.get(o),f,h;if(!n&&(f=e._edges.get(a),f)){if((f.source.key!==i||f.target.key!==o)&&(!r||f.source.key!==o||f.target.key!==i))throw new V(`Graph.${t}: inconsistency detected when attempting to merge the "${a}" edge with "${i}" source & "${o}" target vs. ("${f.source.key}", "${f.target.key}").`);h=f}if(!h&&!e.multi&&c&&(h=r?c.undirected[o]:c.out[o]),h){let b=[h.key,!1,!1,!1];if(l?!u:!s)return b;if(l){let y=h.attributes;h.attributes=u(y),e.emit("edgeAttributesUpdated",{type:"replace",key:h.key,attributes:h.attributes})}else ce(h.attributes,s),e.emit("edgeAttributesUpdated",{type:"merge",key:h.key,attributes:h.attributes,data:s});return b}s=s||{},l&&u&&(s=u(s));let g={key:null,undirected:r,source:i,target:o,attributes:s};if(n)a=e._edgeKeyGenerator();else if(a=""+a,e._edges.has(a))throw new V(`Graph.${t}: the "${a}" edge already exists in the graph.`);let m=!1,v=!1;c||(c=Vi(e,i,{}),m=!0,i===o&&(d=c,v=!0)),d||(d=Vi(e,o,{}),v=!0),f=new Lt(r,a,c,d,s),e._edges.set(a,f);let p=i===o;return r?(c.undirectedDegree++,d.undirectedDegree++,p&&(c.undirectedLoops++,e._undirectedSelfLoopCount++)):(c.outDegree++,d.inDegree++,p&&(c.directedLoops++,e._directedSelfLoopCount++)),e.multi?f.attachMulti():f.attach(),r?e._undirectedSize++:e._directedSize++,g.key=a,e.emit("edgeAdded",g),[a,!0,m,v]}function Ct(e,t){e._edges.delete(t.key);let{source:n,target:r,attributes:a}=t,i=t.undirected,o=n===r;i?(n.undirectedDegree--,r.undirectedDegree--,o&&(n.undirectedLoops--,e._undirectedSelfLoopCount--)):(n.outDegree--,r.inDegree--,o&&(n.directedLoops--,e._directedSelfLoopCount--)),e.multi?t.detachMulti():t.detach(),i?e._undirectedSize--:e._directedSize--,e.emit("edgeDropped",{key:t.key,attributes:a,source:n.key,target:r.key,undirected:i})}var te=class e extends qi.EventEmitter{constructor(t){if(super(),t=ce({},Fd,t),typeof t.multi!="boolean")throw new O(`Graph.constructor: invalid 'multi' option. Expecting a boolean but got "${t.multi}".`);if(!kd.has(t.type))throw new O(`Graph.constructor: invalid 'type' option. Should be one of "mixed", "directed" or "undirected" but got "${t.type}".`);if(typeof t.allowSelfLoops!="boolean")throw new O(`Graph.constructor: invalid 'allowSelfLoops' option. Expecting a boolean but got "${t.allowSelfLoops}".`);let n=t.type==="mixed"?Yi:t.type==="directed"?Ki:Zi;we(this,"NodeDataClass",n);let r="geid_"+Cd()+"_",a=0,i=()=>{let o;do o=r+a++;while(this._edges.has(o));return o};we(this,"_attributes",{}),we(this,"_nodes",new Map),we(this,"_edges",new Map),we(this,"_directedSize",0),we(this,"_undirectedSize",0),we(this,"_directedSelfLoopCount",0),we(this,"_undirectedSelfLoopCount",0),we(this,"_edgeKeyGenerator",i),we(this,"_options",t),$i.forEach(o=>we(this,o,this[o])),Fe(this,"order",()=>this._nodes.size),Fe(this,"size",()=>this._edges.size),Fe(this,"directedSize",()=>this._directedSize),Fe(this,"undirectedSize",()=>this._undirectedSize),Fe(this,"selfLoopCount",()=>this._directedSelfLoopCount+this._undirectedSelfLoopCount),Fe(this,"directedSelfLoopCount",()=>this._directedSelfLoopCount),Fe(this,"undirectedSelfLoopCount",()=>this._undirectedSelfLoopCount),Fe(this,"multi",this._options.multi),Fe(this,"type",this._options.type),Fe(this,"allowSelfLoops",this._options.allowSelfLoops),Fe(this,"implementation",()=>"graphology")}_resetInstanceCounters(){this._directedSize=0,this._undirectedSize=0,this._directedSelfLoopCount=0,this._undirectedSelfLoopCount=0}hasNode(t){return this._nodes.has(""+t)}hasDirectedEdge(t,n){if(this.type==="undirected")return!1;if(arguments.length===1){let r=""+t,a=this._edges.get(r);return!!a&&!a.undirected}else if(arguments.length===2){t=""+t,n=""+n;let r=this._nodes.get(t);return r?r.out.hasOwnProperty(n):!1}throw new O(`Graph.hasDirectedEdge: invalid arity (${arguments.length}, instead of 1 or 2). You can either ask for an edge id or for the existence of an edge between a source & a target.`)}hasUndirectedEdge(t,n){if(this.type==="directed")return!1;if(arguments.length===1){let r=""+t,a=this._edges.get(r);return!!a&&a.undirected}else if(arguments.length===2){t=""+t,n=""+n;let r=this._nodes.get(t);return r?r.undirected.hasOwnProperty(n):!1}throw new O(`Graph.hasDirectedEdge: invalid arity (${arguments.length}, instead of 1 or 2). You can either ask for an edge id or for the existence of an edge between a source & a target.`)}hasEdge(t,n){if(arguments.length===1){let r=""+t;return this._edges.has(r)}else if(arguments.length===2){t=""+t,n=""+n;let r=this._nodes.get(t);return r?typeof r.out<"u"&&r.out.hasOwnProperty(n)||typeof r.undirected<"u"&&r.undirected.hasOwnProperty(n):!1}throw new O(`Graph.hasEdge: invalid arity (${arguments.length}, instead of 1 or 2). You can either ask for an edge id or for the existence of an edge between a source & a target.`)}directedEdge(t,n){if(this.type==="undirected")return;if(t=""+t,n=""+n,this.multi)throw new V("Graph.directedEdge: this method is irrelevant with multigraphs since there might be multiple edges between source & target. See #.directedEdges instead.");let r=this._nodes.get(t);if(!r)throw new W(`Graph.directedEdge: could not find the "${t}" source node in the graph.`);if(!this._nodes.has(n))throw new W(`Graph.directedEdge: could not find the "${n}" target node in the graph.`);let a=r.out&&r.out[n]||void 0;if(a)return a.key}undirectedEdge(t,n){if(this.type==="directed")return;if(t=""+t,n=""+n,this.multi)throw new V("Graph.undirectedEdge: this method is irrelevant with multigraphs since there might be multiple edges between source & target. See #.undirectedEdges instead.");let r=this._nodes.get(t);if(!r)throw new W(`Graph.undirectedEdge: could not find the "${t}" source node in the graph.`);if(!this._nodes.has(n))throw new W(`Graph.undirectedEdge: could not find the "${n}" target node in the graph.`);let a=r.undirected&&r.undirected[n]||void 0;if(a)return a.key}edge(t,n){if(this.multi)throw new V("Graph.edge: this method is irrelevant with multigraphs since there might be multiple edges between source & target. See #.edges instead.");t=""+t,n=""+n;let r=this._nodes.get(t);if(!r)throw new W(`Graph.edge: could not find the "${t}" source node in the graph.`);if(!this._nodes.has(n))throw new W(`Graph.edge: could not find the "${n}" target node in the graph.`);let a=r.out&&r.out[n]||r.undirected&&r.undirected[n]||void 0;if(a)return a.key}areDirectedNeighbors(t,n){t=""+t,n=""+n;let r=this._nodes.get(t);if(!r)throw new W(`Graph.areDirectedNeighbors: could not find the "${t}" node in the graph.`);return this.type==="undirected"?!1:n in r.in||n in r.out}areOutNeighbors(t,n){t=""+t,n=""+n;let r=this._nodes.get(t);if(!r)throw new W(`Graph.areOutNeighbors: could not find the "${t}" node in the graph.`);return this.type==="undirected"?!1:n in r.out}areInNeighbors(t,n){t=""+t,n=""+n;let r=this._nodes.get(t);if(!r)throw new W(`Graph.areInNeighbors: could not find the "${t}" node in the graph.`);return this.type==="undirected"?!1:n in r.in}areUndirectedNeighbors(t,n){t=""+t,n=""+n;let r=this._nodes.get(t);if(!r)throw new W(`Graph.areUndirectedNeighbors: could not find the "${t}" node in the graph.`);return this.type==="directed"?!1:n in r.undirected}areNeighbors(t,n){t=""+t,n=""+n;let r=this._nodes.get(t);if(!r)throw new W(`Graph.areNeighbors: could not find the "${t}" node in the graph.`);return this.type!=="undirected"&&(n in r.in||n in r.out)||this.type!=="directed"&&n in r.undirected}areInboundNeighbors(t,n){t=""+t,n=""+n;let r=this._nodes.get(t);if(!r)throw new W(`Graph.areInboundNeighbors: could not find the "${t}" node in the graph.`);return this.type!=="undirected"&&n in r.in||this.type!=="directed"&&n in r.undirected}areOutboundNeighbors(t,n){t=""+t,n=""+n;let r=this._nodes.get(t);if(!r)throw new W(`Graph.areOutboundNeighbors: could not find the "${t}" node in the graph.`);return this.type!=="undirected"&&n in r.out||this.type!=="directed"&&n in r.undirected}inDegree(t){t=""+t;let n=this._nodes.get(t);if(!n)throw new W(`Graph.inDegree: could not find the "${t}" node in the graph.`);return this.type==="undirected"?0:n.inDegree}outDegree(t){t=""+t;let n=this._nodes.get(t);if(!n)throw new W(`Graph.outDegree: could not find the "${t}" node in the graph.`);return this.type==="undirected"?0:n.outDegree}directedDegree(t){t=""+t;let n=this._nodes.get(t);if(!n)throw new W(`Graph.directedDegree: could not find the "${t}" node in the graph.`);return this.type==="undirected"?0:n.inDegree+n.outDegree}undirectedDegree(t){t=""+t;let n=this._nodes.get(t);if(!n)throw new W(`Graph.undirectedDegree: could not find the "${t}" node in the graph.`);return this.type==="directed"?0:n.undirectedDegree}inboundDegree(t){t=""+t;let n=this._nodes.get(t);if(!n)throw new W(`Graph.inboundDegree: could not find the "${t}" node in the graph.`);let r=0;return this.type!=="directed"&&(r+=n.undirectedDegree),this.type!=="undirected"&&(r+=n.inDegree),r}outboundDegree(t){t=""+t;let n=this._nodes.get(t);if(!n)throw new W(`Graph.outboundDegree: could not find the "${t}" node in the graph.`);let r=0;return this.type!=="directed"&&(r+=n.undirectedDegree),this.type!=="undirected"&&(r+=n.outDegree),r}degree(t){t=""+t;let n=this._nodes.get(t);if(!n)throw new W(`Graph.degree: could not find the "${t}" node in the graph.`);let r=0;return this.type!=="directed"&&(r+=n.undirectedDegree),this.type!=="undirected"&&(r+=n.inDegree+n.outDegree),r}inDegreeWithoutSelfLoops(t){t=""+t;let n=this._nodes.get(t);if(!n)throw new W(`Graph.inDegreeWithoutSelfLoops: could not find the "${t}" node in the graph.`);return this.type==="undirected"?0:n.inDegree-n.directedLoops}outDegreeWithoutSelfLoops(t){t=""+t;let n=this._nodes.get(t);if(!n)throw new W(`Graph.outDegreeWithoutSelfLoops: could not find the "${t}" node in the graph.`);return this.type==="undirected"?0:n.outDegree-n.directedLoops}directedDegreeWithoutSelfLoops(t){t=""+t;let n=this._nodes.get(t);if(!n)throw new W(`Graph.directedDegreeWithoutSelfLoops: could not find the "${t}" node in the graph.`);return this.type==="undirected"?0:n.inDegree+n.outDegree-n.directedLoops*2}undirectedDegreeWithoutSelfLoops(t){t=""+t;let n=this._nodes.get(t);if(!n)throw new W(`Graph.undirectedDegreeWithoutSelfLoops: could not find the "${t}" node in the graph.`);return this.type==="directed"?0:n.undirectedDegree-n.undirectedLoops*2}inboundDegreeWithoutSelfLoops(t){t=""+t;let n=this._nodes.get(t);if(!n)throw new W(`Graph.inboundDegreeWithoutSelfLoops: could not find the "${t}" node in the graph.`);let r=0,a=0;return this.type!=="directed"&&(r+=n.undirectedDegree,a+=n.undirectedLoops*2),this.type!=="undirected"&&(r+=n.inDegree,a+=n.directedLoops),r-a}outboundDegreeWithoutSelfLoops(t){t=""+t;let n=this._nodes.get(t);if(!n)throw new W(`Graph.outboundDegreeWithoutSelfLoops: could not find the "${t}" node in the graph.`);let r=0,a=0;return this.type!=="directed"&&(r+=n.undirectedDegree,a+=n.undirectedLoops*2),this.type!=="undirected"&&(r+=n.outDegree,a+=n.directedLoops),r-a}degreeWithoutSelfLoops(t){t=""+t;let n=this._nodes.get(t);if(!n)throw new W(`Graph.degreeWithoutSelfLoops: could not find the "${t}" node in the graph.`);let r=0,a=0;return this.type!=="directed"&&(r+=n.undirectedDegree,a+=n.undirectedLoops*2),this.type!=="undirected"&&(r+=n.inDegree+n.outDegree,a+=n.directedLoops*2),r-a}source(t){t=""+t;let n=this._edges.get(t);if(!n)throw new W(`Graph.source: could not find the "${t}" edge in the graph.`);return n.source.key}target(t){t=""+t;let n=this._edges.get(t);if(!n)throw new W(`Graph.target: could not find the "${t}" edge in the graph.`);return n.target.key}extremities(t){t=""+t;let n=this._edges.get(t);if(!n)throw new W(`Graph.extremities: could not find the "${t}" edge in the graph.`);return[n.source.key,n.target.key]}opposite(t,n){t=""+t,n=""+n;let r=this._edges.get(n);if(!r)throw new W(`Graph.opposite: could not find the "${n}" edge in the graph.`);let a=r.source.key,i=r.target.key;if(t===a)return i;if(t===i)return a;throw new W(`Graph.opposite: the "${t}" node is not attached to the "${n}" edge (${a}, ${i}).`)}hasExtremity(t,n){t=""+t,n=""+n;let r=this._edges.get(t);if(!r)throw new W(`Graph.hasExtremity: could not find the "${t}" edge in the graph.`);return r.source.key===n||r.target.key===n}isUndirected(t){t=""+t;let n=this._edges.get(t);if(!n)throw new W(`Graph.isUndirected: could not find the "${t}" edge in the graph.`);return n.undirected}isDirected(t){t=""+t;let n=this._edges.get(t);if(!n)throw new W(`Graph.isDirected: could not find the "${t}" edge in the graph.`);return!n.undirected}isSelfLoop(t){t=""+t;let n=this._edges.get(t);if(!n)throw new W(`Graph.isSelfLoop: could not find the "${t}" edge in the graph.`);return n.source===n.target}addNode(t,n){return Pd(this,t,n).key}mergeNode(t,n){if(n&&!he(n))throw new O(`Graph.mergeNode: invalid attributes. Expecting an object but got "${n}"`);t=""+t,n=n||{};let r=this._nodes.get(t);return r?(n&&(ce(r.attributes,n),this.emit("nodeAttributesUpdated",{type:"merge",key:t,attributes:r.attributes,data:n})),[t,!1]):(r=new this.NodeDataClass(t,n),this._nodes.set(t,r),this.emit("nodeAdded",{key:t,attributes:n}),[t,!0])}updateNode(t,n){if(n&&typeof n!="function")throw new O(`Graph.updateNode: invalid updater function. Expecting a function but got "${n}"`);t=""+t;let r=this._nodes.get(t);if(r){if(n){let i=r.attributes;r.attributes=n(i),this.emit("nodeAttributesUpdated",{type:"replace",key:t,attributes:r.attributes})}return[t,!1]}let a=n?n({}):{};return r=new this.NodeDataClass(t,a),this._nodes.set(t,r),this.emit("nodeAdded",{key:t,attributes:a}),[t,!0]}dropNode(t){t=""+t;let n=this._nodes.get(t);if(!n)throw new W(`Graph.dropNode: could not find the "${t}" node in the graph.`);let r;if(this.type!=="undirected"){for(let a in n.out){r=n.out[a];do Ct(this,r),r=r.next;while(r)}for(let a in n.in){r=n.in[a];do Ct(this,r),r=r.next;while(r)}}if(this.type!=="directed")for(let a in n.undirected){r=n.undirected[a];do Ct(this,r),r=r.next;while(r)}this._nodes.delete(t),this.emit("nodeDropped",{key:t,attributes:n.attributes})}dropEdge(t){let n;if(arguments.length>1){let r=""+arguments[0],a=""+arguments[1];if(n=De(this,r,a,this.type),!n)throw new W(`Graph.dropEdge: could not find the "${r}" -> "${a}" edge in the graph.`)}else if(t=""+t,n=this._edges.get(t),!n)throw new W(`Graph.dropEdge: could not find the "${t}" edge in the graph.`);return Ct(this,n),this}dropDirectedEdge(t,n){if(arguments.length<2)throw new V("Graph.dropDirectedEdge: it does not make sense to try and drop a directed edge by key. What if the edge with this key is undirected? Use #.dropEdge for this purpose instead.");if(this.multi)throw new V("Graph.dropDirectedEdge: cannot use a {source,target} combo when dropping an edge in a MultiGraph since we cannot infer the one you want to delete as there could be multiple ones.");t=""+t,n=""+n;let r=De(this,t,n,"directed");if(!r)throw new W(`Graph.dropDirectedEdge: could not find a "${t}" -> "${n}" edge in the graph.`);return Ct(this,r),this}dropUndirectedEdge(t,n){if(arguments.length<2)throw new V("Graph.dropUndirectedEdge: it does not make sense to drop a directed edge by key. What if the edge with this key is undirected? Use #.dropEdge for this purpose instead.");if(this.multi)throw new V("Graph.dropUndirectedEdge: cannot use a {source,target} combo when dropping an edge in a MultiGraph since we cannot infer the one you want to delete as there could be multiple ones.");let r=De(this,t,n,"undirected");if(!r)throw new W(`Graph.dropUndirectedEdge: could not find a "${t}" -> "${n}" edge in the graph.`);return Ct(this,r),this}clear(){this._edges.clear(),this._nodes.clear(),this._resetInstanceCounters(),this.emit("cleared")}clearEdges(){let t=this._nodes.values(),n;for(;n=t.next(),n.done!==!0;)n.value.clear();this._edges.clear(),this._resetInstanceCounters(),this.emit("edgesCleared")}getAttribute(t){return this._attributes[t]}getAttributes(){return this._attributes}hasAttribute(t){return this._attributes.hasOwnProperty(t)}setAttribute(t,n){return this._attributes[t]=n,this.emit("attributesUpdated",{type:"set",attributes:this._attributes,name:t}),this}updateAttribute(t,n){if(typeof n!="function")throw new O("Graph.updateAttribute: updater should be a function.");let r=this._attributes[t];return this._attributes[t]=n(r),this.emit("attributesUpdated",{type:"set",attributes:this._attributes,name:t}),this}removeAttribute(t){return delete this._attributes[t],this.emit("attributesUpdated",{type:"remove",attributes:this._attributes,name:t}),this}replaceAttributes(t){if(!he(t))throw new O("Graph.replaceAttributes: provided attributes are not a plain object.");return this._attributes=t,this.emit("attributesUpdated",{type:"replace",attributes:this._attributes}),this}mergeAttributes(t){if(!he(t))throw new O("Graph.mergeAttributes: provided attributes are not a plain object.");return ce(this._attributes,t),this.emit("attributesUpdated",{type:"merge",attributes:this._attributes,data:t}),this}updateAttributes(t){if(typeof t!="function")throw new O("Graph.updateAttributes: provided updater is not a function.");return this._attributes=t(this._attributes),this.emit("attributesUpdated",{type:"update",attributes:this._attributes}),this}updateEachNodeAttributes(t,n){if(typeof t!="function")throw new O("Graph.updateEachNodeAttributes: expecting an updater function.");if(n&&!ji(n))throw new O("Graph.updateEachNodeAttributes: invalid hints. Expecting an object having the following shape: {attributes?: [string]}");let r=this._nodes.values(),a,i;for(;a=r.next(),a.done!==!0;)i=a.value,i.attributes=t(i.key,i.attributes);this.emit("eachNodeAttributesUpdated",{hints:n||null})}updateEachEdgeAttributes(t,n){if(typeof t!="function")throw new O("Graph.updateEachEdgeAttributes: expecting an updater function.");if(n&&!ji(n))throw new O("Graph.updateEachEdgeAttributes: invalid hints. Expecting an object having the following shape: {attributes?: [string]}");let r=this._edges.values(),a,i,o,s;for(;a=r.next(),a.done!==!0;)i=a.value,o=i.source,s=i.target,i.attributes=t(i.key,i.attributes,o.key,s.key,o.attributes,s.attributes,i.undirected);this.emit("eachEdgeAttributesUpdated",{hints:n||null})}forEachAdjacencyEntry(t){if(typeof t!="function")throw new O("Graph.forEachAdjacencyEntry: expecting a callback.");jn(!1,!1,!1,this,t)}forEachAdjacencyEntryWithOrphans(t){if(typeof t!="function")throw new O("Graph.forEachAdjacencyEntryWithOrphans: expecting a callback.");jn(!1,!1,!0,this,t)}forEachAssymetricAdjacencyEntry(t){if(typeof t!="function")throw new O("Graph.forEachAssymetricAdjacencyEntry: expecting a callback.");jn(!1,!0,!1,this,t)}forEachAssymetricAdjacencyEntryWithOrphans(t){if(typeof t!="function")throw new O("Graph.forEachAssymetricAdjacencyEntryWithOrphans: expecting a callback.");jn(!1,!0,!0,this,t)}nodes(){return Array.from(this._nodes.keys())}forEachNode(t){if(typeof t!="function")throw new O("Graph.forEachNode: expecting a callback.");let n=this._nodes.values(),r,a;for(;r=n.next(),r.done!==!0;)a=r.value,t(a.key,a.attributes)}findNode(t){if(typeof t!="function")throw new O("Graph.findNode: expecting a callback.");let n=this._nodes.values(),r,a;for(;r=n.next(),r.done!==!0;)if(a=r.value,t(a.key,a.attributes))return a.key}mapNodes(t){if(typeof t!="function")throw new O("Graph.mapNode: expecting a callback.");let n=this._nodes.values(),r,a,i=new Array(this.order),o=0;for(;r=n.next(),r.done!==!0;)a=r.value,i[o++]=t(a.key,a.attributes);return i}someNode(t){if(typeof t!="function")throw new O("Graph.someNode: expecting a callback.");let n=this._nodes.values(),r,a;for(;r=n.next(),r.done!==!0;)if(a=r.value,t(a.key,a.attributes))return!0;return!1}everyNode(t){if(typeof t!="function")throw new O("Graph.everyNode: expecting a callback.");let n=this._nodes.values(),r,a;for(;r=n.next(),r.done!==!0;)if(a=r.value,!t(a.key,a.attributes))return!1;return!0}filterNodes(t){if(typeof t!="function")throw new O("Graph.filterNodes: expecting a callback.");let n=this._nodes.values(),r,a,i=[];for(;r=n.next(),r.done!==!0;)a=r.value,t(a.key,a.attributes)&&i.push(a.key);return i}reduceNodes(t,n){if(typeof t!="function")throw new O("Graph.reduceNodes: expecting a callback.");if(arguments.length<2)throw new O("Graph.reduceNodes: missing initial value. You must provide it because the callback takes more than one argument and we cannot infer the initial value from the first iteration, as you could with a simple array.");let r=n,a=this._nodes.values(),i,o;for(;i=a.next(),i.done!==!0;)o=i.value,r=t(r,o.key,o.attributes);return r}nodeEntries(){let t=this._nodes.values();return{[Symbol.iterator](){return this},next(){let n=t.next();if(n.done)return n;let r=n.value;return{value:{node:r.key,attributes:r.attributes},done:!1}}}}export(){let t=new Array(this._nodes.size),n=0;this._nodes.forEach((a,i)=>{t[n++]=Rd(i,a)});let r=new Array(this._edges.size);return n=0,this._edges.forEach((a,i)=>{r[n++]=Ad(this.type,i,a)}),{options:{type:this.type,multi:this.multi,allowSelfLoops:this.allowSelfLoops},attributes:this.getAttributes(),nodes:t,edges:r}}import(t,n=!1){if(t instanceof e)return t.forEachNode((l,u)=>{n?this.mergeNode(l,u):this.addNode(l,u)}),t.forEachEdge((l,u,c,d,f,h,g)=>{n?g?this.mergeUndirectedEdgeWithKey(l,c,d,u):this.mergeDirectedEdgeWithKey(l,c,d,u):g?this.addUndirectedEdgeWithKey(l,c,d,u):this.addDirectedEdgeWithKey(l,c,d,u)}),this;if(!he(t))throw new O("Graph.import: invalid argument. Expecting a serialized graph or, alternatively, a Graph instance.");if(t.attributes){if(!he(t.attributes))throw new O("Graph.import: invalid attributes. Expecting a plain object.");n?this.mergeAttributes(t.attributes):this.replaceAttributes(t.attributes)}let r,a,i,o,s;if(t.nodes){if(i=t.nodes,!Array.isArray(i))throw new O("Graph.import: invalid nodes. Expecting an array.");for(r=0,a=i.length;r<a;r++){o=i[r],wd(o);let{key:l,attributes:u}=o;n?this.mergeNode(l,u):this.addNode(l,u)}}if(t.edges){let l=!1;if(this.type==="undirected"&&(l=!0),i=t.edges,!Array.isArray(i))throw new O("Graph.import: invalid edges. Expecting an array.");for(r=0,a=i.length;r<a;r++){s=i[r],Dd(s);let{source:u,target:c,attributes:d,undirected:f=l}=s,h;"key"in s?(h=n?f?this.mergeUndirectedEdgeWithKey:this.mergeDirectedEdgeWithKey:f?this.addUndirectedEdgeWithKey:this.addDirectedEdgeWithKey,h.call(this,s.key,u,c,d)):(h=n?f?this.mergeUndirectedEdge:this.mergeDirectedEdge:f?this.addUndirectedEdge:this.addDirectedEdge,h.call(this,u,c,d))}}return this}nullCopy(t){let n=new e(ce({},this._options,t));return n.replaceAttributes(ce({},this.getAttributes())),n}emptyCopy(t){let n=this.nullCopy(t);return this._nodes.forEach((r,a)=>{let i=ce({},r.attributes);r=new n.NodeDataClass(a,i),n._nodes.set(a,r)}),n}copy(t){if(t=t||{},typeof t.type=="string"&&t.type!==this.type&&t.type!=="mixed")throw new V(`Graph.copy: cannot create an incompatible copy from "${this.type}" type to "${t.type}" because this would mean losing information about the current graph.`);if(typeof t.multi=="boolean"&&t.multi!==this.multi&&t.multi!==!0)throw new V("Graph.copy: cannot create an incompatible copy by downgrading a multi graph to a simple one because this would mean losing information about the current graph.");if(typeof t.allowSelfLoops=="boolean"&&t.allowSelfLoops!==this.allowSelfLoops&&t.allowSelfLoops!==!0)throw new V("Graph.copy: cannot create an incompatible copy from a graph allowing self loops to one that does not because this would mean losing information about the current graph.");let n=this.emptyCopy(t),r=this._edges.values(),a,i;for(;a=r.next(),a.done!==!0;)i=a.value,no(n,"copy",!1,i.undirected,i.key,i.source.key,i.target.key,ce({},i.attributes));return n}toJSON(){return this.export()}toString(){return"[object Graph]"}inspect(){let t={};this._nodes.forEach((i,o)=>{t[o]=i.attributes});let n={},r={};this._edges.forEach((i,o)=>{let s=i.undirected?"--":"->",l="",u=i.source.key,c=i.target.key,d;i.undirected&&u>c&&(d=u,u=c,c=d);let f=`(${u})${s}(${c})`;o.startsWith("geid_")?this.multi&&(typeof r[f]>"u"?r[f]=0:r[f]++,l+=`${r[f]}. `):l+=`[${o}]: `,l+=f,n[l]=i.attributes});let a={};for(let i in this)this.hasOwnProperty(i)&&!$i.has(i)&&typeof this[i]!="function"&&typeof i!="symbol"&&(a[i]=this[i]);return a.attributes=this._attributes,a.nodes=t,a.edges=n,we(a,"constructor",this.constructor),a}};typeof Symbol<"u"&&(te.prototype[Symbol.for("nodejs.util.inspect.custom")]=te.prototype.inspect);Ld.forEach(e=>{["add","merge","update"].forEach(t=>{let n=e.name(t),r=t==="add"?no:Id;e.generateKey?te.prototype[n]=function(a,i,o){return r(this,n,!0,(e.type||this.type)==="undirected",null,a,i,o,t==="update")}:te.prototype[n]=function(a,i,o,s){return r(this,n,!1,(e.type||this.type)==="undirected",a,i,o,s,t==="update")}})});Hc(te);ed(te);vd(te);Sd(te);var $n=class extends te{constructor(t){let n=ce({type:"directed"},t);if("multi"in n&&n.multi!==!1)throw new O("DirectedGraph.from: inconsistent indication that the graph should be multi in given options!");if(n.type!=="directed")throw new O('DirectedGraph.from: inconsistent "'+n.type+'" type in given options!');super(n)}},Vn=class extends te{constructor(t){let n=ce({type:"undirected"},t);if("multi"in n&&n.multi!==!1)throw new O("UndirectedGraph.from: inconsistent indication that the graph should be multi in given options!");if(n.type!=="undirected")throw new O('UndirectedGraph.from: inconsistent "'+n.type+'" type in given options!');super(n)}},qn=class extends te{constructor(t){let n=ce({multi:!0},t);if("multi"in n&&n.multi!==!0)throw new O("MultiGraph.from: inconsistent indication that the graph should be simple in given options!");super(n)}},Xn=class extends te{constructor(t){let n=ce({type:"directed",multi:!0},t);if("multi"in n&&n.multi!==!0)throw new O("MultiDirectedGraph.from: inconsistent indication that the graph should be simple in given options!");if(n.type!=="directed")throw new O('MultiDirectedGraph.from: inconsistent "'+n.type+'" type in given options!');super(n)}},Yn=class extends te{constructor(t){let n=ce({type:"undirected",multi:!0},t);if("multi"in n&&n.multi!==!0)throw new O("MultiUndirectedGraph.from: inconsistent indication that the graph should be simple in given options!");if(n.type!=="undirected")throw new O('MultiUndirectedGraph.from: inconsistent "'+n.type+'" type in given options!');super(n)}};function Ft(e){e.from=function(t,n){let r=ce({},t.options,n),a=new e(r);return a.import(t),a}}Ft(te);Ft($n);Ft(Vn);Ft(qn);Ft(Xn);Ft(Yn);te.Graph=te;te.DirectedGraph=$n;te.UndirectedGraph=Vn;te.MultiGraph=qn;te.MultiDirectedGraph=Xn;te.MultiUndirectedGraph=Yn;te.InvalidArgumentsGraphError=O;te.NotFoundGraphError=W;te.UsageGraphError=V;function Jn(e,t){(t==null||t>e.length)&&(t=e.length);for(var n=0,r=Array(t);n<t;n++)r[n]=e[n];return r}function Qn(e,t){if(e){if(typeof e=="string")return Jn(e,t);var n={}.toString.call(e).slice(8,-1);return n==="Object"&&e.constructor&&(n=e.constructor.name),n==="Map"||n==="Set"?Array.from(e):n==="Arguments"||/^(?:Ui|I)nt(?:8|16|32)(?:Clamped)?Array$/.test(n)?Jn(e,t):void 0}}function Nd(e){if(Array.isArray(e))return e}function zd(e,t){var n=e==null?null:typeof Symbol<"u"&&e[Symbol.iterator]||e["@@iterator"];if(n!=null){var r,a,i,o,s=[],l=!0,u=!1;try{if(i=(n=n.call(e)).next,t===0){if(Object(n)!==n)return;l=!1}else for(;!(l=(r=i.call(n)).done)&&(s.push(r.value),s.length!==t);l=!0);}catch(c){u=!0,a=c}finally{try{if(!l&&n.return!=null&&(o=n.return(),Object(o)!==o))return}finally{if(u)throw a}}return s}}function Md(){throw new TypeError(`Invalid attempt to destructure non-iterable instance.
In order to be iterable, non-array objects must have a [Symbol.iterator]() method.`)}function Q(e,t){return Nd(e)||zd(e,t)||Qn(e,t)||Md()}function G(e,t){var n=typeof Symbol<"u"&&e[Symbol.iterator]||e["@@iterator"];if(!n){if(Array.isArray(e)||(n=Qn(e))||t&&e&&typeof e.length=="number"){n&&(e=n);var r=0,a=function(){};return{s:a,n:function(){return r>=e.length?{done:!0}:{done:!1,value:e[r++]}},e:function(l){throw l},f:a}}throw new TypeError(`Invalid attempt to iterate non-iterable instance.
In order to be iterable, non-array objects must have a [Symbol.iterator]() method.`)}var i,o=!0,s=!1;return{s:function(){n=n.call(e)},n:function(){var l=n.next();return o=l.done,l},e:function(l){s=!0,i=l},f:function(){try{o||n.return==null||n.return()}finally{if(s)throw i}}}}function _e(){return Float32Array.of(1,0,0,0,1,0,0,0,1)}function sn(e,t,n){return e[0]=t,e[4]=typeof n=="number"?n:t,e}function ca(e,t){var n=Math.sin(t),r=Math.cos(t);return e[0]=r,e[1]=n,e[3]=-n,e[4]=r,e}function da(e,t,n){return e[6]=t,e[7]=n,e}function Ye(e,t){var n=e[0],r=e[1],a=e[2],i=e[3],o=e[4],s=e[5],l=e[6],u=e[7],c=e[8],d=t[0],f=t[1],h=t[2],g=t[3],m=t[4],v=t[5],p=t[6],b=t[7],y=t[8];return e[0]=d*n+f*i+h*l,e[1]=d*r+f*o+h*u,e[2]=d*a+f*s+h*c,e[3]=g*n+m*i+v*l,e[4]=g*r+m*o+v*u,e[5]=g*a+m*s+v*c,e[6]=p*n+b*i+y*l,e[7]=p*r+b*o+y*u,e[8]=p*a+b*s+y*c,e}function er(e,t){var n=Math.cos(t),r=Math.sin(t);return{x:n*e.x+r*e.y,y:-r*e.x+n*e.y}}function ht(e,t){var n=arguments.length>2&&arguments[2]!==void 0?arguments[2]:1,r=e[0],a=e[1],i=e[3],o=e[4],s=e[6],l=e[7],u=t.x,c=t.y;return{x:u*r+c*i+s*n,y:u*a+c*o+l*n}}function ro(e,t){var n=e.height/e.width,r=t.height/t.width;return n<1&&r>1||n>1&&r<1?1:Math.min(Math.max(r,1/r),Math.max(1/n,n))}function Ke(e,t,n,r,a){var i=e.angle,o=e.ratio,s=e.x,l=e.y,u=t.width,c=t.height,d=_e(),f=Math.min(u,c)-2*r,h=ro(t,n);return a?(Ye(d,da(_e(),s,l)),Ye(d,sn(_e(),o)),Ye(d,ca(_e(),i)),Ye(d,sn(_e(),u/f/2/h,c/f/2/h))):(Ye(d,sn(_e(),2*(f/u)*h,2*(f/c)*h)),Ye(d,ca(_e(),-i)),Ye(d,sn(_e(),1/o)),Ye(d,da(_e(),-s,-l))),d}function ha(e,t,n){var r=ht(e,{x:Math.cos(t.angle),y:Math.sin(t.angle)},0),a=r.x,i=r.y;return 1/Math.sqrt(Math.pow(a,2)+Math.pow(i,2))/n.width}function tr(e,t,n,r,a){var i=Math.floor(t/a*r),o=Math.floor(e.drawingBufferHeight/a-n/a*r);return[i,o]}var Pt={black:"#000000",silver:"#C0C0C0",gray:"#808080",grey:"#808080",white:"#FFFFFF",maroon:"#800000",red:"#FF0000",purple:"#800080",fuchsia:"#FF00FF",green:"#008000",lime:"#00FF00",olive:"#808000",yellow:"#FFFF00",navy:"#000080",blue:"#0000FF",teal:"#008080",aqua:"#00FFFF",darkblue:"#00008B",mediumblue:"#0000CD",darkgreen:"#006400",darkcyan:"#008B8B",deepskyblue:"#00BFFF",darkturquoise:"#00CED1",mediumspringgreen:"#00FA9A",springgreen:"#00FF7F",cyan:"#00FFFF",midnightblue:"#191970",dodgerblue:"#1E90FF",lightseagreen:"#20B2AA",forestgreen:"#228B22",seagreen:"#2E8B57",darkslategray:"#2F4F4F",darkslategrey:"#2F4F4F",limegreen:"#32CD32",mediumseagreen:"#3CB371",turquoise:"#40E0D0",royalblue:"#4169E1",steelblue:"#4682B4",darkslateblue:"#483D8B",mediumturquoise:"#48D1CC",indigo:"#4B0082",darkolivegreen:"#556B2F",cadetblue:"#5F9EA0",cornflowerblue:"#6495ED",rebeccapurple:"#663399",mediumaquamarine:"#66CDAA",dimgray:"#696969",dimgrey:"#696969",slateblue:"#6A5ACD",olivedrab:"#6B8E23",slategray:"#708090",slategrey:"#708090",lightslategray:"#778899",lightslategrey:"#778899",mediumslateblue:"#7B68EE",lawngreen:"#7CFC00",chartreuse:"#7FFF00",aquamarine:"#7FFFD4",skyblue:"#87CEEB",lightskyblue:"#87CEFA",blueviolet:"#8A2BE2",darkred:"#8B0000",darkmagenta:"#8B008B",saddlebrown:"#8B4513",darkseagreen:"#8FBC8F",lightgreen:"#90EE90",mediumpurple:"#9370DB",darkviolet:"#9400D3",palegreen:"#98FB98",darkorchid:"#9932CC",yellowgreen:"#9ACD32",sienna:"#A0522D",brown:"#A52A2A",darkgray:"#A9A9A9",darkgrey:"#A9A9A9",lightblue:"#ADD8E6",greenyellow:"#ADFF2F",paleturquoise:"#AFEEEE",lightsteelblue:"#B0C4DE",powderblue:"#B0E0E6",firebrick:"#B22222",darkgoldenrod:"#B8860B",mediumorchid:"#BA55D3",rosybrown:"#BC8F8F",darkkhaki:"#BDB76B",mediumvioletred:"#C71585",indianred:"#CD5C5C",peru:"#CD853F",chocolate:"#D2691E",tan:"#D2B48C",lightgray:"#D3D3D3",lightgrey:"#D3D3D3",thistle:"#D8BFD8",orchid:"#DA70D6",goldenrod:"#DAA520",palevioletred:"#DB7093",crimson:"#DC143C",gainsboro:"#DCDCDC",plum:"#DDA0DD",burlywood:"#DEB887",lightcyan:"#E0FFFF",lavender:"#E6E6FA",darksalmon:"#E9967A",violet:"#EE82EE",palegoldenrod:"#EEE8AA",lightcoral:"#F08080",khaki:"#F0E68C",aliceblue:"#F0F8FF",honeydew:"#F0FFF0",azure:"#F0FFFF",sandybrown:"#F4A460",wheat:"#F5DEB3",beige:"#F5F5DC",whitesmoke:"#F5F5F5",mintcream:"#F5FFFA",ghostwhite:"#F8F8FF",salmon:"#FA8072",antiquewhite:"#FAEBD7",linen:"#FAF0E6",lightgoldenrodyellow:"#FAFAD2",oldlace:"#FDF5E6",magenta:"#FF00FF",deeppink:"#FF1493",orangered:"#FF4500",tomato:"#FF6347",hotpink:"#FF69B4",coral:"#FF7F50",darkorange:"#FF8C00",lightsalmon:"#FFA07A",orange:"#FFA500",lightpink:"#FFB6C1",pink:"#FFC0CB",gold:"#FFD700",peachpuff:"#FFDAB9",navajowhite:"#FFDEAD",moccasin:"#FFE4B5",bisque:"#FFE4C4",mistyrose:"#FFE4E1",blanchedalmond:"#FFEBCD",papayawhip:"#FFEFD5",lavenderblush:"#FFF0F5",seashell:"#FFF5EE",cornsilk:"#FFF8DC",lemonchiffon:"#FFFACD",floralwhite:"#FFFAF0",snow:"#FFFAFA",lightyellow:"#FFFFE0",ivory:"#FFFFF0"},ao=new Int8Array(4),Zn=new Int32Array(ao.buffer,0,1),io=new Float32Array(ao.buffer,0,1),Gd=/^\s*rgba?\s*\(/,Bd=/^\s*rgba?\s*\(\s*([0-9]*)\s*,\s*([0-9]*)\s*,\s*([0-9]*)(?:\s*,\s*(.*)?)?\)\s*$/;function ln(e){var t=0,n=0,r=0,a=1,i=e.toLowerCase();if(i==="transparent")return{r:0,g:0,b:0,a:0};if(i in Pt)return ln(Pt[i]);if(e[0]==="#")e.length===4?(t=parseInt(e.charAt(1)+e.charAt(1),16),n=parseInt(e.charAt(2)+e.charAt(2),16),r=parseInt(e.charAt(3)+e.charAt(3),16)):(t=parseInt(e.charAt(1)+e.charAt(2),16),n=parseInt(e.charAt(3)+e.charAt(4),16),r=parseInt(e.charAt(5)+e.charAt(6),16)),e.length===9&&(a=parseInt(e.charAt(7)+e.charAt(8),16)/255);else if(Gd.test(e)){var o=e.match(Bd);o&&(t=+o[1],n=+o[2],r=+o[3],o[4]&&(a=+o[4]))}return{r:t,g:n,b:r,a}}function un(e){var t=arguments.length>1&&arguments[1]!==void 0?arguments[1]:!1,n=ln(e),r=n.r,a=n.g,i=n.b,o=n.a;return t?[r/255*o,a/255*o,i/255*o,o]:[r/255,a/255,i/255,o]}var It={};for(on in Pt)It[on]=xe(Pt[on]),It[Pt[on]]=It[on];var on;function oo(e,t,n,r,a){return Zn[0]=r<<24|n<<16|t<<8|e,a&&(Zn[0]=Zn[0]&4278190079),io[0]}function xe(e){if(e=e.toLowerCase(),typeof It[e]<"u")return It[e];var t=ln(e),n=t.r,r=t.g,a=t.b,i=t.a;i=i*255|0;var o=oo(n,r,a,i,!0);return It[e]=o,o}function Ze(e,t){io[0]=xe(e);var n=Zn[0];t&&(n=n|16777216);var r=n&255,a=n>>8&255,i=n>>16&255,o=n>>24&255;return[r,a,i,o]}function ft(e){var t=e>>>8&255,n=e>>>16&255,r=e>>>24&255,a=e&255;return(a<<24|r<<16|n<<8|t)>>>0}function nr(e,t,n,r){return(n<<24|t<<16|e<<8|r)>>>0}function fa(e,t,n,r,a,i){var o=tr(e,n,r,a,i),s=Q(o,2),l=s[0],u=s[1],c=new Uint8Array(4);e.bindFramebuffer(e.FRAMEBUFFER,t),e.readPixels(l,u,1,1,e.RGBA,e.UNSIGNED_BYTE,c);var d=Q(c,4),f=d[0],h=d[1],g=d[2],m=d[3];return[f,h,g,m]}function ga(e){var t=ln(e),n=t.r,r=t.g,a=t.b,i=t.a,o=(n/255).toFixed(6),s=(r/255).toFixed(6),l=(a/255).toFixed(6),u=i.toFixed(6);return"vec4(".concat(o,", ").concat(s,", ").concat(l,", ").concat(u,")")}function Wd(e,t){if(typeof e!="object"||!e)return e;var n=e[Symbol.toPrimitive];if(n!==void 0){var r=n.call(e,t||"default");if(typeof r!="object")return r;throw new TypeError("@@toPrimitive must return a primitive value.")}return(t==="string"?String:Number)(e)}function uo(e){var t=Wd(e,"string");return typeof t=="symbol"?t:t+""}function R(e,t,n){return(t=uo(t))in e?Object.defineProperty(e,t,{value:n,enumerable:!0,configurable:!0,writable:!0}):e[t]=n,e}function so(e,t){var n=Object.keys(e);if(Object.getOwnPropertySymbols){var r=Object.getOwnPropertySymbols(e);t&&(r=r.filter(function(a){return Object.getOwnPropertyDescriptor(e,a).enumerable})),n.push.apply(n,r)}return n}function B(e){for(var t=1;t<arguments.length;t++){var n=arguments[t]!=null?arguments[t]:{};t%2?so(Object(n),!0).forEach(function(r){R(e,r,n[r])}):Object.getOwnPropertyDescriptors?Object.defineProperties(e,Object.getOwnPropertyDescriptors(n)):so(Object(n)).forEach(function(r){Object.defineProperty(e,r,Object.getOwnPropertyDescriptor(n,r))})}return e}function K(e,t){if(!(e instanceof t))throw new TypeError("Cannot call a class as a function")}function lo(e,t){for(var n=0;n<t.length;n++){var r=t[n];r.enumerable=r.enumerable||!1,r.configurable=!0,"value"in r&&(r.writable=!0),Object.defineProperty(e,uo(r.key),r)}}function Z(e,t,n){return t&&lo(e.prototype,t),n&&lo(e,n),Object.defineProperty(e,"prototype",{writable:!1}),e}function gt(e){return gt=Object.setPrototypeOf?Object.getPrototypeOf.bind():function(t){return t.__proto__||Object.getPrototypeOf(t)},gt(e)}function co(){try{var e=!Boolean.prototype.valueOf.call(Reflect.construct(Boolean,[],function(){}))}catch{}return(co=function(){return!!e})()}function Od(e){if(e===void 0)throw new ReferenceError("this hasn't been initialised - super() hasn't been called");return e}function Ud(e,t){if(t&&(typeof t=="object"||typeof t=="function"))return t;if(t!==void 0)throw new TypeError("Derived constructors may only return object or undefined");return Od(e)}function ae(e,t,n){return t=gt(t),Ud(e,co()?Reflect.construct(t,n||[],gt(e).constructor):t.apply(e,n))}function pa(e,t){return pa=Object.setPrototypeOf?Object.setPrototypeOf.bind():function(n,r){return n.__proto__=r,n},pa(e,t)}function ie(e,t){if(typeof t!="function"&&t!==null)throw new TypeError("Super expression must either be null or a function");e.prototype=Object.create(t&&t.prototype,{constructor:{value:e,writable:!0,configurable:!0}}),Object.defineProperty(e,"prototype",{writable:!1}),t&&pa(e,t)}function Hd(e){if(Array.isArray(e))return Jn(e)}function jd(e){if(typeof Symbol<"u"&&e[Symbol.iterator]!=null||e["@@iterator"]!=null)return Array.from(e)}function $d(){throw new TypeError(`Invalid attempt to spread non-iterable instance.
In order to be iterable, non-array objects must have a [Symbol.iterator]() method.`)}function $(e){return Hd(e)||jd(e)||Qn(e)||$d()}function J(e){"@babel/helpers - typeof";return J=typeof Symbol=="function"&&typeof Symbol.iterator=="symbol"?function(t){return typeof t}:function(t){return t&&typeof Symbol=="function"&&t.constructor===Symbol&&t!==Symbol.prototype?"symbol":typeof t},J(e)}function ze(e){return J(e)==="object"&&e!==null&&"attribute"in e}function Nt(){var e=`
float sdf_circle(vec2 uv, float size) {
  return length(uv) - size;
}
`;return{name:"circle",glsl:e,uniforms:[]}}function cn(e){var t,n=WebGL2RenderingContext,r=n.UNSIGNED_BYTE,a=(t=e?.color)!==null&&t!==void 0?t:{attribute:"color"};if(!ze(a)){var i=a,o=`
vec4 layer_fill() {
  return `.concat(ga(i),`;
}
`);return{name:"fill",uniforms:[],attributes:[],glsl:o}}var s=a.attribute,l=`
vec4 layer_fill(vec4 v_fillColor) {
  return v_fillColor;
}
`;return{name:"fill",uniforms:[],attributes:[{name:"fillColor",size:4,type:r,normalized:!0,source:s}],glsl:l}}function ho(e,t){if(typeof e=="string"){var n=un(e).map(function(u){return u.toFixed(6)}),r=Q(n,4),a=r[0],i=r[1],o=r[2],s=r[3];return{glsl:"vec4(".concat(a,", ").concat(i,", ").concat(o,", ").concat(s,")"),attributes:[],needsNodeColors:!1}}if("node"in e&&e.node)return{glsl:e.node==="source"?"v_sourceColor":"v_targetColor",attributes:[],needsNodeColors:!0};var l=e;return{glsl:"v_".concat(t),attributes:[{name:"a_".concat(t),size:4,type:WebGL2RenderingContext.UNSIGNED_BYTE,normalized:!0,source:l.attribute,defaultValue:l.default}],needsNodeColors:!1}}function Me(e){var t=e?.color;if(t===void 0){var n=`
// Plain solid color layer
vec4 layer_plain(EdgeContext ctx) {
  return v_color;
}
`;return{name:"plain",glsl:n,uniforms:[],attributes:[]}}var r=ho(t,"plainColor"),a=`
// Plain solid color layer
vec4 layer_plain(EdgeContext ctx) {
  return `.concat(r.glsl,`;
}
`);return{name:"plain",glsl:a,uniforms:[],attributes:r.attributes,needsNodeColors:r.needsNodeColors}}function dn(){var e=`
// Position at parameter t \u2208 [0, 1]
vec2 path_straight_position(float t, vec2 source, vec2 target) {
  return mix(source, target, t);
}

// Total length of the path (analytical - more efficient than sampling)
float path_straight_length(vec2 source, vec2 target) {
  return length(target - source);
}
`;return{name:"straight",segments:1,minBodyLengthRatio:0,linearParameterization:!0,glsl:e,uniforms:[],attributes:[]}}function hn(e){var t=e??{},n=t.segments,r=n===void 0?32:n,a=`
const float LOOP_PI = 3.141592653589793;

float loopWorldRadius() {
  float nodeWorldRadius = v_sourceNodeSize * u_correctionRatio / u_sizeRatio;
  return max(v_loopRadius * nodeWorldRadius, 0.001);
}

// Cubic B\xE9zier with P0 = P3 = source.
// Control points extend outward orthogonal to the node surface at exit/entry angles.
vec2 path_loop_position(float t, vec2 source, vec2 target) {
  float R = loopWorldRadius();
  float angle = v_loopAngle + (v_loopFixedOrientation > 0.5 ? u_cameraAngle : 0.0);
  float halfSpread = v_loopSpread * 0.5;

  float exitAngle = angle - halfSpread;
  float entryAngle = angle + halfSpread;

  // Control point distance: at t=0.5 the B\xE9zier reaches 0.75 * cpDist * cos(halfSpread)
  // from source. Solve for cpDist so the loop tip reaches exactly R.
  float cpDist = R / (0.75 * cos(halfSpread));

  vec2 cp1 = source + cpDist * vec2(cos(exitAngle), sin(exitAngle));
  vec2 cp2 = source + cpDist * vec2(cos(entryAngle), sin(entryAngle));

  float u = 1.0 - t;
  vec2 d1 = cp1 - source;
  vec2 d2 = cp2 - source;
  return source + 3.0 * t * u * (u * d1 + t * d2);
}

// Approximate arc length via chord sampling
float path_loop_length(vec2 source, vec2 target) {
  float len = 0.0;
  vec2 prev = path_loop_position(0.0, source, target);
  const int STEPS = 16;
  for (int i = 1; i <= STEPS; i++) {
    float t = float(i) / float(STEPS);
    vec2 cur = path_loop_position(t, source, target);
    len += length(cur - prev);
    prev = cur;
  }
  return len;
}
`;return{name:"loop",segments:r,glsl:a,needsNodeSize:!0,uniforms:[],attributes:[{name:"loopRadius",size:1,type:WebGL2RenderingContext.FLOAT},{name:"loopAngle",size:1,type:WebGL2RenderingContext.FLOAT},{name:"loopSpread",size:1,type:WebGL2RenderingContext.FLOAT},{name:"loopFixedOrientation",size:1,type:WebGL2RenderingContext.FLOAT}],variables:{loopRadius:{type:"number",default:4},loopAngle:{type:"number",default:Math.PI/4},loopSpread:{type:"number",default:80*Math.PI/180},loopFixedOrientation:{type:"number",default:0}},spread:{variable:"loopRadius",compute:function(o){return o+4}}}}var bo=Ne(dt());function fo(e){return J(e)==="object"&&"glsl"in e&&!("attributes"in e)}function go(e){return J(e)==="object"&&"glsl"in e&&!("attributes"in e)}var fn={shapes:[Nt()],variables:{},layers:[cn()],label:{},backdrop:{},labelAttachments:{}},zt={paths:[dn(),hn()],extremities:[],variables:{},layers:[Me()],defaultHead:"none",defaultTail:"none",label:{}},rr=["nodes","topNodes"],ar=["edges","topEdges"],ir=[].concat(ar,rr),po={nodes:fn,edges:zt,depthLayers:$(ir)};var Vd=function(t){return t},qd=function(t){return t*t},Xd=function(t){return t*(2-t)},Yd=function(t){return(t*=2)<1?.5*t*t:-.5*(--t*(t-2)-1)},Kd=function(t){return t*t*t},Zd=function(t){return--t*t*t+1},Jd=function(t){return(t*=2)<1?.5*t*t*t:.5*((t-=2)*t*t+2)},Qd=function(t){return t===0?0:Math.pow(2,10*(t-1))},eh=function(t){return t===1?1:1-Math.pow(2,-10*t)},th=function(t){return t===0?0:t===1?1:t<.5?Math.pow(2,10*(2*t-1))/2:(2-Math.pow(2,-10*(2*t-1)))/2},va={linear:Vd,quadraticIn:qd,quadraticOut:Xd,quadraticInOut:Yd,cubicIn:Kd,cubicOut:Zd,cubicInOut:Jd,exponentialIn:Qd,exponentialOut:eh,exponentialInOut:th};function or(e){return e?typeof e=="function"?e:va[e]:va.linear}function ba(e){return J(e)==="object"&&e!==null&&"attribute"in e}function yo(e){return typeof e=="function"}function _o(e){return J(e)==="object"&&e!==null&&"when"in e&&typeof e.when=="function"&&"then"in e}function xo(e){return J(e)==="object"&&e!==null&&"whenState"in e&&"then"in e}function To(e){return J(e)==="object"&&e!==null&&"whenData"in e&&"then"in e}var nh={isHovered:!1,isLabelHovered:!1,isHidden:!1,isHighlighted:!1,isDragged:!1},rh={isHovered:!1,isLabelHovered:!1,isHidden:!1,isHighlighted:!1,parallelIndex:0,parallelCount:1},ah={isIdle:!0,isPanning:!1,isZooming:!1,isDragging:!1,hasHovered:!1,hasHighlighted:!1};function Eo(e){return B(B({},nh),e)}function So(e){return B(B({},rh),e)}function ya(e){return B(B({},ah),e)}var ih={x:{attribute:"x"},y:{attribute:"y"},size:{whenState:"isHovered",then:{attribute:"size",defaultValue:12},else:{attribute:"size",defaultValue:10}},color:{attribute:"color",defaultValue:"#666"},label:{attribute:"label"},visibility:{whenState:"isHidden",then:"hidden",else:"visible"},labelVisibility:{whenState:"isHovered",then:"visible",else:"auto"},backdropVisibility:{whenState:"isHovered",then:"visible",else:"hidden"},backdropColor:"#ffffff",backdropShadowColor:"rgba(0, 0, 0, 0.5)",backdropShadowBlur:12,backdropPadding:6},oh={size:{attribute:"size",defaultValue:1},color:{attribute:"color",defaultValue:"#ccc"},label:{attribute:"label"},visibility:{whenState:"isHidden",then:"hidden",else:"visible"}};var lr={nodes:B(B({},ih),{},{depth:{whenState:"isHovered",then:"topNodes",else:"nodes"}}),edges:B(B({},oh),{},{depth:{whenState:["isHighlighted","isHovered"],then:"topEdges",else:"edges"}})};function sh(e){return"min"in e||"max"in e||"minValue"in e||"maxValue"in e||"easing"in e}function lh(e){return"dict"in e}function ur(e,t){return typeof e=="string"?t[e]===!0:Array.isArray(e)?e.every(function(n){return t[n]===!0}):J(e)==="object"&&e!==null?Object.entries(e).every(function(n){var r=Q(n,2),a=r[0],i=r[1];return t[a]===i}):!1}function uh(e,t){var n=t[e.attribute];return n===void 0?e.defaultValue:n}function ch(e,t,n){var r,a,i,o,s=t[e.attribute];if(s===void 0)return e.defaultValue;var l=Number(s);if(isNaN(l))return e.defaultValue;if(e.min===void 0&&e.max===void 0)return l;var u=(r=e.minValue)!==null&&r!==void 0?r:l,c=(a=e.maxValue)!==null&&a!==void 0?a:l;if(c===u){var d;return(d=e.min)!==null&&d!==void 0?d:l}var f=(l-u)/(c-u);f=Math.max(0,Math.min(1,f));var h=or(e.easing);f=h(f);var g=(i=e.min)!==null&&i!==void 0?i:0,m=(o=e.max)!==null&&o!==void 0?o:1;return g+f*(m-g)}function dh(e,t){var n=t[e.attribute];if(n===void 0)return e.defaultValue;var r=String(n);return r in e.dict?e.dict[r]:e.defaultValue}function hh(e,t,n){return lh(e)?dh(e,t):sh(e)?ch(e,t):uh(e,t)}function Ro(e,t){return typeof e=="string"?!!t[e]:Array.isArray(e)?e.every(function(n){return!!t[n]}):J(e)==="object"&&e!==null?Object.entries(e).every(function(n){var r=Q(n,2),a=r[0],i=r[1];return t[a]===i}):!1}function sr(e,t,n,r,a,i){if(e==null)return i;if(J(e)!=="object"&&typeof e!="function")return e;if(_o(e)){var o=e.when(t,n,r,a)?e.then:e.else;return o===void 0?i:sr(o,t,n,r,a,i)}if(xo(e)){var s=ur(e.whenState,n)?e.then:e.else;return s===void 0?i:sr(s,t,n,r,a,i)}if(To(e)){var l=Ro(e.whenData,t)?e.then:e.else;return l===void 0?i:sr(l,t,n,r,a,i)}if(yo(e)){var u=e(t,n,r,a);return u??i}if(ba(e)){var c=hh(e,t);return c??i}return e}function Pe(e){if(e==null)return"static";if(_o(e))return"graph-state";if(xo(e)){var t=Pe(e.then),n=e.else!==void 0?Pe(e.else):"static";return Ce("item-state",Ce(t,n))}if(To(e)){var r=Pe(e.then),a=e.else!==void 0?Pe(e.else):"static";return Ce(r,a)}return yo(e)?"graph-state":(ba(e),"static")}function Ce(e,t){return e==="graph-state"||t==="graph-state"?"graph-state":e==="item-state"||t==="item-state"?"item-state":"static"}function _a(e){if(!e)return{dependency:"static",xAttribute:null,yAttribute:null};var t=Array.isArray(e)?e:[e],n="static",r=null,a=null,i=G(t),o;try{for(i.s();!(o=i.n()).done;){var s=o.value;if("matchData"in s&&"cases"in s)for(var l=s.cases,u=0,c=Object.values(l);u<c.length;u++)for(var d=c[u],f=0,h=Object.values(d);f<h.length;f++){var g=h[f];n=Ce(n,Pe(g))}else if("matchState"in s&&"cases"in s){n=Ce(n,"item-state");for(var m=s.cases,v=0,p=Object.values(m);v<p.length;v++)for(var b=p[v],y=0,_=Object.values(b);y<_.length;y++){var x=_[y];n=Ce(n,Pe(x))}}else if("when"in s&&"then"in s)n="graph-state";else if("whenState"in s){n=Ce(n,"item-state");var T=s.then;if(T&&J(T)==="object")for(var E=0,S=Object.values(T);E<S.length;E++){var w=S[E];n=Ce(n,Pe(w))}var A=s.else;if(A&&J(A)==="object")for(var C=0,D=Object.values(A);C<D.length;C++){var F=D[C];n=Ce(n,Pe(F))}}else if("whenData"in s){var k=s.then;if(k&&J(k)==="object")for(var L=0,z=Object.values(k);L<z.length;L++){var N=z[L];n=Ce(n,Pe(N))}var P=s.else;if(P&&J(P)==="object")for(var I=0,M=Object.values(P);I<M.length;I++){var j=M[I];n=Ce(n,Pe(j))}}else for(var U=0,Y=Object.entries(s);U<Y.length;U++){var q=Q(Y[U],2),X=q[0],Le=q[1];if(!Ta.has(X)&&(n=Ce(n,Pe(Le)),(X==="x"||X==="y")&&ba(Le))){var ye=Le.attribute;X==="x"&&!r&&(r=ye),X==="y"&&!a&&(a=ye)}}}}catch(Ve){i.e(Ve)}finally{i.f()}return{dependency:n,xAttribute:r,yAttribute:a}}var Je={size:10,color:"#666",opacity:1,shape:"circle",rotationAlignment:"viewport",labelRotationAlignment:"viewport",visibility:"visible",depth:"nodes",zIndex:0,label:"",labelColor:"#000",labelSize:12,labelFont:"sans-serif",labelVisibility:"auto",labelPosition:"right",labelAngle:0,labelDepth:"nodes",backdropVisibility:"hidden",backdropColor:"transparent",backdropShadowColor:"transparent",backdropShadowBlur:0,backdropPadding:0,backdropBorderColor:"transparent",backdropBorderWidth:0,backdropCornerRadius:0,backdropLabelPadding:-1,backdropArea:"both",labelAttachment:null,labelAttachmentPlacement:"below"},xa={size:1,color:"#ccc",opacity:1,path:"straight",selfLoopPath:"loop",parallelSpread:.25,tail:"none",head:"none",visibility:"visible",depth:"edges",zIndex:0,label:"",labelColor:"#666",labelVisibility:"auto",labelDepth:"edges"},Ta=new Set(["when","whenState","whenData","then","else"]),vo=Object.keys(Je),fh=Object.values(Je),mo=Object.keys(xa),gh=Object.values(xa);function Mt(e,t,n,r,a,i,o){for(var s in t){var l;if(!Ta.has(s)){var u=(l=e[s])!==null&&l!==void 0?l:o[s];e[s]=sr(t[s],n,r,a,i,u)}}"depth"in t&&!("labelDepth"in t)&&(e.labelDepth=e.depth)}function Ao(e,t,n,r,a,i,o){var s=G(t),l;try{for(s.s();!(l=s.n()).done;){var u=l.value;if("matchData"in u&&"cases"in u){var c,d=String((c=n[u.matchData])!==null&&c!==void 0?c:""),f=u.cases;d in f&&Mt(e,f[d],n,r,a,i,o)}else if("matchState"in u&&"cases"in u){var h,g=String((h=r[u.matchState])!==null&&h!==void 0?h:""),m=u.cases;g in m&&Mt(e,m[g],n,r,a,i,o)}else if("when"in u&&"then"in u){var v=u.when;if(!v(n,r,a,i))continue;Mt(e,u.then,n,r,a,i,o)}else if("whenState"in u&&"then"in u){if(!ur(u.whenState,r))continue;Mt(e,u.then,n,r,a,i,o)}else if("whenData"in u&&"then"in u){if(!Ro(u.whenData,n))continue;Mt(e,u.then,n,r,a,i,o)}else Mt(e,u,n,r,a,i,o)}}catch(p){s.e(p)}finally{s.f()}}function Ea(e,t,n,r,a,i){for(var o=i||{},s=o,l=0,u=vo.length;l<u;l++)s[vo[l]]=fh[l];if(s.x=void 0,s.y=void 0,s.labelBackgroundColor=void 0,s.labelBackgroundPadding=void 0,s.labelCursor=void 0,!e){var c,d,f,h,g;return s.x=(c=t.x)!==null&&c!==void 0?c:0,s.y=(d=t.y)!==null&&d!==void 0?d:0,o.size=(f=t.size)!==null&&f!==void 0?f:10,o.color=(h=t.color)!==null&&h!==void 0?h:"#666",o.label=(g=t.label)!==null&&g!==void 0?g:"",o}var m=Array.isArray(e)?e:[e];return Ao(s,m,t,n,r,a,Je),o}function Sa(e,t,n,r,a,i){for(var o=i||{},s=o,l=0,u=mo.length;l<u;l++)s[mo[l]]=gh[l];if(!e){var c,d,f;return o.size=(c=t.size)!==null&&c!==void 0?c:1,o.color=(d=t.color)!==null&&d!==void 0?d:"#ccc",o.label=(f=t.label)!==null&&f!==void 0?f:"",o}var h=Array.isArray(e)?e:[e];return Ao(s,h,t,n,r,a,xa),typeof s.labelPosition=="number"&&(s.labelPosition=void 0),o}function ph(e,t,n){var r,a;if(e==null)return n;if(typeof e=="function")return(r=e(t))!==null&&r!==void 0?r:n;if(J(e)==="object"&&"when"in e){var i,o=e,s=o.when(t),l=s?o.then:o.else;return l===void 0?n:typeof l=="function"?(i=l(t))!==null&&i!==void 0?i:n:l??n}if(J(e)==="object"&&"whenState"in e){var u,c=e,d=ur(c.whenState,t),f=d?c.then:c.else;return f===void 0?n:typeof f=="function"?(u=f(t))!==null&&u!==void 0?u:n:f??n}return(a=e)!==null&&a!==void 0?a:n}function Ra(e,t){var n={};if(!e)return n;var r=Array.isArray(e)?e:[e],a=G(r),i;try{for(a.s();!(i=a.n()).done;){var o=i.value;if("when"in o&&"then"in o){var s=o.when(t),l=s?o.then:o.else;l&&J(l)==="object"&&ma(n,l,t)}else if("whenState"in o&&"then"in o){var u=ur(o.whenState,t),c=u?o.then:o.else;c&&J(c)==="object"&&ma(n,c,t)}else ma(n,o,t)}}catch(d){a.e(d)}finally{a.f()}return n}function ma(e,t,n){for(var r=0,a=Object.entries(t);r<a.length;r++){var i=Q(a[r],2),o=i[0],s=i[1];Ta.has(o)||(e[o]=ph(s,n))}}var cr=(function(e){function t(){var n;return K(this,t),n=ae(this,t),n.rawEmitter=n,n}return ie(t,e),Z(t)})(bo.EventEmitter);var wo=po;function Aa(e,t){var n=t.size;if(n!==0){var r=e.length;e.length+=n;var a=0;t.forEach(function(i){e[r+a]=i,a++})}}function Do(e){e=e||{};for(var t=0,n=arguments.length<=1?0:arguments.length-1;t<n;t++){var r=t+1<1||arguments.length<=t+1?void 0:arguments[t+1];r&&Object.assign(e,r)}return e}function Gt(e,t){for(var n in t)if(Object.prototype.hasOwnProperty.call(t,n)&&t[n]!==e[n])return!0;return!1}function Co(e,t){var n=e,r=t;for(var a in n)if(Object.prototype.hasOwnProperty.call(n,a)&&n[a]!==r[a])return!1;for(var i in r)if(Object.prototype.hasOwnProperty.call(r,i)&&r[i]!==n[i])return!1;return!0}function pt(e,t,n){n?e.add(t):e.delete(t)}var Lo=Ne(Bt());var Fo={easing:"quadraticInOut",duration:150};function Qe(e){return e.labelVisibility==="visible"&&e.visibility!=="hidden"}function wa(e){return e.backdropVisibility==="visible"&&e.visibility!=="hidden"}function dr(e){return[e.rotationAlignment==="graph"?1:0,e.labelRotationAlignment==="graph"?1:0]}function Da(e,t,n){var r=e[t];if(r)for(var a=0;a<r.length;a++){var i=r[a];if(!(n<i.offset||n>=i.offset+i.count)){if(i.count===1)r.splice(a,1);else if(n===i.offset)i.offset++,i.count--;else if(n===i.offset+i.count-1)i.count--;else{var o=n+1,s=i.offset+i.count-o;i.count=n-i.offset,r.splice(a+1,0,{offset:o,count:s})}return}}}function Ca(e,t,n){if(!e[t]){e[t]=[{offset:n,count:1}];return}for(var r=e[t],a=r.length,i=0;i<r.length;i++)if(n<r[i].offset){a=i;break}var o=a>0?r[a-1]:null,s=a<r.length?r[a]:null,l=o&&o.offset+o.count===n,u=s&&n+1===s.offset;l&&u?(o.count+=1+s.count,r.splice(a,1)):l?o.count++:u?(s.offset--,s.count++):r.splice(a,0,{offset:n,count:1})}function Po(e){if(!(0,Lo.default)(e))throw new Error("Sigma: invalid graph instance.")}function hr(e){var t=Q(e.x,2),n=t[0],r=t[1],a=Q(e.y,2),i=a[0],o=a[1],s=Math.max(r-n,o-i),l=(r+n)/2,u=(o+i)/2;(s===0||Math.abs(s)===1/0||isNaN(s))&&(s=1),isNaN(l)&&(l=0),isNaN(u)&&(u=0);var c=function(f){return{x:.5+(f.x-l)/s,y:.5+(f.y-u)/s}};return c.applyTo=function(d){d.x=.5+(d.x-l)/s,d.y=.5+(d.y-u)/s},c.inverse=function(d){return{x:l+s*(d.x-.5),y:u+s*(d.y-.5)}},c.ratio=s,c}var fr={hideEdgesOnMove:!1,hideLabelsOnMove:!1,renderLabels:!0,renderEdgeLabels:!1,edgeLabelAnchors:"nodeLabels",enableEdgeEvents:!1,nodeLabelEvents:!1,edgeLabelEvents:!1,pickingDownSizingRatio:2,nodePickingPadding:0,edgePickingPadding:4,labelPickingPadding:10,stagePadding:30,minEdgeThickness:1.7,antiAliasingFeather:1,antialiasEdges:!0,antialiasNodes:!0,dragTimeout:100,draggedEventsTolerance:3,inertiaDuration:200,inertiaRatio:3,zoomDuration:250,zoomingRatio:1.7,doubleClickTimeout:300,doubleClickZoomingRatio:2.2,doubleClickZoomingDuration:200,tapMoveTolerance:10,zoomToSizeRatioFunction:Math.sqrt,itemSizesReference:"positions",autoRescale:!0,autoRescaleContent:"positions",enableNodeDrag:!1,getDraggedNodes:function(t){return[t]},dragPositionToAttributes:null,labelDensity:1,labelGridCellSize:100,labelRenderedSizeThreshold:6,labelPixelSnapping:!0,minCameraRatio:null,maxCameraRatio:null,enableCameraZooming:!0,enableCameraPanning:!0,enableCameraRotation:!0,enableCameraMouseRotation:!0,cameraPanBoundaries:null,gestureTarget:"graph",sharedGestureWheelMessage:"Use Ctrl + scroll to zoom the graph",sharedGestureAppleWheelMessage:"Use \u2318 + scroll to zoom the graph",sharedGestureTouchMessage:"Use two fingers to move the graph",allowInvalidContainer:!1,DEBUG_displayPickingLayer:!1,DEBUG_logShaders:!1,DEBUG_logRenderStats:!1,DEBUG_gpuTimerQueries:!1};function gr(e){if(typeof e.labelDensity!="number"||e.labelDensity<0)throw new Error("Settings: invalid `labelDensity`. Expecting a positive number.");var t=e.minCameraRatio,n=e.maxCameraRatio;if(typeof t=="number"&&typeof n=="number"&&n<t)throw new Error("Settings: invalid camera ratio boundaries. Expecting `maxCameraRatio` to be greater than `minCameraRatio`.")}function Io(e){return Do({},fr,e)}var vh=new Set(["italic","oblique"]),mh=new Set(["bold","bolder","lighter"]);function pr(e){for(var t="normal",n="normal",r=e.trim(),a=r.split(/\s+/),i=0,o=0;o<a.length;o++){var s=a[o].toLowerCase();if(vh.has(s))n=s,i=o+1;else if(mh.has(s)||/^\d{3}$/.test(s))t=s,i=o+1;else if(s==="normal")i=o+1;else break}var l=a.slice(i).join(" ");return{family:l||r,weight:t,style:n}}function Wt(e,t,n){var r=document.createElement(e);if(t)for(var a in t)r.style[a]=t[a];if(n)for(var i in n)r.setAttribute(i,n[i]);return r}function gn(){return typeof window.devicePixelRatio<"u"?window.devicePixelRatio:1}var Oo=Ne(dt());function bh(e){var t=e.type===WebGL2RenderingContext.FLOAT&&!e.normalized,n=e.type===WebGL2RenderingContext.UNSIGNED_BYTE&&e.size===4&&e.normalized;if(!t&&!n)throw new Error('Attribute "'.concat(e.name,'" is invalid: only non-normalized FLOAT and normalized UNSIGNED_BYTE with size 4 are supported.'));return e.normalized?1:e.size}function xr(e){var t=0;return e.forEach(function(n){return t+=bh(n)}),t}function Uo(e,t,n){var r=e==="VERTEX"?t.VERTEX_SHADER:t.FRAGMENT_SHADER,a=t.createShader(r);if(a===null)throw new Error("loadShader: error while creating the shader");t.shaderSource(a,n),t.compileShader(a);var i=t.getShaderParameter(a,t.COMPILE_STATUS);if(!i){var o=t.getShaderInfoLog(a);throw t.deleteShader(a),new Error(`loadShader: error while compiling the shader:
`.concat(o,`
`).concat(n))}return a}function Er(e,t){return Uo("VERTEX",e,t)}function Sr(e,t){return Uo("FRAGMENT",e,t)}function Rr(e,t){var n=e.createProgram();if(n===null)throw new Error("loadProgram: error while creating the program.");var r,a;for(r=0,a=t.length;r<a;r++)e.attachShader(n,t[r]);e.linkProgram(n);var i=e.getProgramParameter(n,e.LINK_STATUS);if(!i){var o=e.getProgramInfoLog(n);throw e.deleteProgram(n),new Error("loadProgram: error while linking the program: ".concat(o))}return n}function ka(e){var t=e.gl,n=e.buffer,r=e.program,a=e.vertexShader,i=e.fragmentShader;t.deleteShader(a),t.deleteShader(i),t.deleteProgram(r),t.deleteBuffer(n)}function re(e){return e%1===0?e.toFixed(1):e.toString()}var Tr=new Map,Ho=new Map,yh=0;function Ia(e,t){var n,r=(n=e.variables)===null||n===void 0||(n=n[t.source||t.name.replace(/^a_/,"")])===null||n===void 0?void 0:n.default,a=typeof r=="number"?r:typeof t.defaultValue=="number"?t.defaultValue:0;return re(a)}function yn(e,t,n){var r,a=arguments.length>3&&arguments[3]!==void 0?arguments[3]:function(o){return"v_".concat(o.name.replace(/^a_/,""))},i=[t,n].concat($(e.uniforms.filter(function(o){return o.type==="float"}).map(function(o){var s;return re((s=o.value)!==null&&s!==void 0?s:0)})),$(((r=e.attributes)!==null&&r!==void 0?r:[]).map(a)));return"sdf_".concat(e.name,"(").concat(i.join(", "),")")}function _h(e){var t,n=e.name,r=e.uniforms.filter(function(o){return o.type==="float"&&o.value!==void 0&&o.value!==0}).map(function(o){return"".concat(o.name.replace("u_",""),"=").concat(o.value)}).sort(),a=((t=e.attributes)!==null&&t!==void 0?t:[]).map(function(o){var s;return"".concat(o.name,"@").concat((s=o.source)!==null&&s!==void 0?s:"","=").concat(Ia(e,o))}),i=[].concat($(r),$(a));return i.length>0&&(n+="#"+i.join("#")),n}function jo(e){var t=_h(e);if(!Tr.has(t)){var n={},r=G(e.uniforms),a;try{for(r.s();!(a=r.n()).done;){var i=a.value;i.type==="float"&&i.value!==void 0&&(n[i.name]=i.value)}}catch(o){r.e(o)}finally{r.f()}Tr.set(t,{shape:e,uniformValues:n,slug:t}),Ho.set(t,yh++)}return t}function Ut(e){var t;return(t=Ho.get(e))!==null&&t!==void 0?t:-1}function $o(e){var t=[],n=new Set,r=new Set,a=/mat2 rotate2D\(float angle\)\s*\{[^}]+\}/,i=G(e),o;try{for(i.s();!(o=i.n()).done;){var s=o.value;if(!r.has(s.name)){r.add(s.name);var l=s.glsl;a.test(l)&&(n.has("rotate2D")?l=l.replace(a,""):n.add("rotate2D")),t.push(l)}}}catch(u){i.e(u)}finally{i.f()}return t.join(`
`)}function Vo(){return $o(Array.from(Tr.values()).map(function(e){return e.shape}))}function qo(e){var t=Array.from(Tr.entries());if(t.length===0)return`
float querySDF(int shapeId, vec2 uv, float size) {
  return length(uv) - size;
}
`;var n=t.map(function(r,a){var i=Q(r,2),o=i[0],s=i[1].shape,l=yn(s,"uv","size",function(u){var c=u.name.replace(/^a_/,"");return e!=null&&e.has(c)?"g_".concat(c):Ia(s,u)});return"    case ".concat(a,": return ").concat(l,"; // ").concat(o)}).join(`
`);return`
float querySDF(int shapeId, vec2 uv, float size) {
  switch (shapeId) {
`.concat(n,`
    default: return length(uv) - size;
  }
}
`)}function Ar(e){return $o(e)}function Ht(e){return $(new Map(e.flatMap(function(t){return t.uniforms}).map(function(t){return[t.name,t]})).values())}function Xo(e,t){if(e.length===0)return`
void queryNodeSDF(int shapeId, vec2 uv, float size) {
  context.sdf = length(uv) - size;
  context.inradiusFactor = 1.0;
}
`;var n=function(d){return yn(d,"uv","size")},r=function(d){var f,h;return(f=d.inradiusFactorGLSL)!==null&&f!==void 0?f:re((h=d.inradiusFactor)!==null&&h!==void 0?h:1)};if(e.length===1){var a=e[0];return`
void queryNodeSDF(int shapeId, vec2 uv, float size) {
  context.sdf = `.concat(n(a),`;
  context.inradiusFactor = `).concat(r(a),`;
}
`)}var i=e.map(function(c,d){return"    case ".concat(d,": // ").concat(c.name,`
      context.sdf = `).concat(n(c),`;
      context.inradiusFactor = `).concat(r(c),`;
      break;`)}).join(`
`),o=e[0],s="",l="shapeId";if(t&&t.length>1){var u=t.map(function(c,d){return"    case ".concat(c,": return ").concat(d,"; // ").concat(e[d].name)}).join(`
`);s=`
int globalToLocalShapeId(int globalId) {
  switch (globalId) {
`.concat(u,`
    default: return 0;
  }
}
`),l="globalToLocalShapeId(shapeId)"}return"".concat(s,`
void queryNodeSDF(int shapeId, vec2 uv, float size) {
  switch (`).concat(l,`) {
`).concat(i,`
    default:
      context.sdf = `).concat(n(o),`;
      context.inradiusFactor = `).concat(r(o),`;
  }
}
`)}var mt={right:0,left:1,above:2,below:3,over:4},bt=5,wr=3,Na=`
float matrixScaleX = length(vec2(u_matrix[0][0], u_matrix[1][0]));
float nodeRadiusGraphSpace = nodeSize * u_correctionRatio / u_sizeRatio * 2.0;
float nodeRadiusNDC = nodeRadiusGraphSpace * matrixScaleX;
float nodeRadiusPixels = nodeRadiusNDC * u_resolution.x / 2.0;
`,Yo=`
mat2 rotate2D(float angle) {
  float c = cos(angle);
  float s = sin(angle);
  return mat2(c, -s, s, c);
}
`,za=`
vec2 getLabelDirection(float positionMode) {
  if (positionMode < 0.5) return vec2(1.0, 0.0);   // Right
  if (positionMode < 1.5) return vec2(-1.0, 0.0);  // Left
  if (positionMode < 2.5) return vec2(0.0, -1.0);  // Above
  if (positionMode < 3.5) return vec2(0.0, 1.0);   // Below
  return vec2(0.0);                                 // Over (centered)
}
`,Ko=`
float sdfBox(vec2 p, vec2 halfSize) {
  vec2 d = abs(p) - halfSize;
  return length(max(d, 0.0)) + min(max(d.x, d.y), 0.0);
}
`,Zo=`
float sdfRotatedBox(vec2 p, vec2 halfSize, float angle) {
  float c = cos(-angle);
  float s = sin(-angle);
  vec2 rotatedP = mat2(c, -s, s, c) * p;
  return sdfBox(rotatedP, halfSize);
}
`,Jo=`
float sdfRoundedBox(vec2 p, vec2 halfSize, float radius) {
  vec2 d = abs(p) - halfSize + radius;
  return length(max(d, 0.0)) + min(max(d.x, d.y), 0.0) - radius;
}
`,Qo=`
float sdfRoundedRotatedBox(vec2 p, vec2 halfSize, float angle, float radius) {
  float c = cos(-angle);
  float s = sin(-angle);
  vec2 rotatedP = mat2(c, -s, s, c) * p;
  return sdfRoundedBox(rotatedP, halfSize, radius);
}
`,Ma=`
vec2 labelBoxCenter(float positionMode, float labelStart, vec2 halfSize, float textHalfY) {
  if (positionMode < 0.5) return vec2(labelStart + halfSize.x, 0.0);    // right
  if (positionMode < 1.5) return vec2(-(labelStart + halfSize.x), 0.0); // left
  if (positionMode < 2.5) return vec2(0.0, -(labelStart + textHalfY));  // above
  if (positionMode < 3.5) return vec2(0.0, labelStart + textHalfY);     // below
  return vec2(0.0);                                                     // over
}
`,Dr=2,Be=`
vec4 readNodeData(sampler2D nodeDataTexture, int nodeDataTextureWidth, int nodeIndex) {
  int t = nodeIndex * `.concat(Dr,`;
  ivec2 coord = ivec2(t % nodeDataTextureWidth, t / nodeDataTextureWidth);
  return texelFetch(nodeDataTexture, coord, 0);
}
`),nt=`
vec4 readNodeFlags(sampler2D nodeDataTexture, int nodeDataTextureWidth, int nodeIndex) {
  int t = nodeIndex * `.concat(Dr,` + 1;
  ivec2 coord = ivec2(t % nodeDataTextureWidth, t / nodeDataTextureWidth);
  return texelFetch(nodeDataTexture, coord, 0);
}
`),es=`
vec4 readNodeColor(sampler2D nodeDataTexture, int nodeDataTextureWidth, int nodeIndex) {
  int t = nodeIndex * `.concat(Dr,` + 1;
  ivec2 coord = ivec2(t % nodeDataTextureWidth, t / nodeDataTextureWidth);
  vec4 texel = texelFetch(nodeDataTexture, coord, 0);
  float r = floor(texel.b / 65536.0);
  float g = floor(mod(texel.b, 65536.0) / 256.0);
  float b = mod(texel.b, 256.0);
  return vec4(r / 255.0, g / 255.0, b / 255.0, texel.a);
}
`),yt=`
vec4 readFrameTexel(sampler2D frameTexture, int frameTextureWidth, int index) {
  ivec2 coord = ivec2(index % frameTextureWidth, index / frameTextureWidth);
  return texelFetch(frameTexture, coord, 0);
}
`;function ts(e,t){var n=function(o){return yn(o,"uv","size")};if(e.length===1)return{code:No(n(e[0])),multiShape:!1};var r=e.map(function(i,o){return"    case ".concat(t?t[o]:o,": return ").concat(n(i),";")}).join(`
`),a=`
float queryShapeSDF(int shapeId, vec2 uv, float size) {
  switch (shapeId) {
`.concat(r,`
    default: return `).concat(n(e[0]),`;
  }
}
int g_shapeId;
`).concat(No("queryShapeSDF(g_shapeId, uv, size)"),`
`);return{code:a,multiShape:!0}}function No(e){return`
float findEdgeDistance(vec2 direction, float size) {
  float lo = 0.0, hi = 2.0;
  for (int i = 0; i < 8; i++) {
    float mid = (lo + hi) * 0.5;
    vec2 uv = direction * mid;
    if (`.concat(e,` < 0.0) lo = mid; else hi = mid;
  }
  return (lo + hi) * 0.5;
}
`)}function xh(e,t){for(;!{}.hasOwnProperty.call(e,t)&&(e=gt(e))!==null;);return e}function La(){return La=typeof Reflect<"u"&&Reflect.get?Reflect.get.bind():function(e,t,n){var r=xh(e,t);if(r){var a=Object.getOwnPropertyDescriptor(r,t);return a.get?a.get.call(arguments.length<3?e:n):a.value}},La.apply(null,arguments)}function de(e,t,n,r){var a=La(gt(1&r?e.prototype:e),t,n);return 2&r&&typeof a=="function"?function(i){return a.apply(n,i)}:a}var Th=1024,Eh=1.5,Sh=4096,ns=-1,_n=(function(){function e(t,n){var r=arguments.length>2&&arguments[2]!==void 0?arguments[2]:Th;K(this,e),R(this,"texture",null),R(this,"dirty",!1),R(this,"dirtyRangeStart",1/0),R(this,"dirtyRangeEnd",-1),R(this,"indexMap",new Map),R(this,"freeIndices",[]),R(this,"nextIndex",0),this.gl=t,this.TEXELS_PER_ITEM=n,this.capacity=this.roundUpToPowerOfTwo(r);var a=this.computeTextureDimensions(this.capacity);this.textureWidth=a.width,this.textureHeight=a.height,this.data=new Float32Array(this.textureWidth*this.textureHeight*4),this.createTexture()}return Z(e,[{key:"computeTextureDimensions",value:function(n){var r=n*this.TEXELS_PER_ITEM,a=Math.min(r,Sh),i=Math.ceil(r/a);return{width:a,height:i}}},{key:"roundUpToPowerOfTwo",value:function(n){return Math.pow(2,Math.ceil(Math.log2(Math.max(1,n))))}},{key:"createTexture",value:function(){var n=this.gl;this.texture=n.createTexture(),n.activeTexture(n.TEXTURE0),n.bindTexture(n.TEXTURE_2D,this.texture),n.texImage2D(n.TEXTURE_2D,0,n.RGBA32F,this.textureWidth,this.textureHeight,0,n.RGBA,n.FLOAT,this.data),n.texParameteri(n.TEXTURE_2D,n.TEXTURE_MIN_FILTER,n.NEAREST),n.texParameteri(n.TEXTURE_2D,n.TEXTURE_MAG_FILTER,n.NEAREST),n.texParameteri(n.TEXTURE_2D,n.TEXTURE_WRAP_S,n.CLAMP_TO_EDGE),n.texParameteri(n.TEXTURE_2D,n.TEXTURE_WRAP_T,n.CLAMP_TO_EDGE),n.bindTexture(n.TEXTURE_2D,null)}},{key:"resize",value:function(n){if(!(n<=this.capacity)){var r=this.roundUpToPowerOfTwo(Math.ceil(n*Eh)),a=this.gl,i=this.computeTextureDimensions(r),o=new Float32Array(i.width*i.height*4);o.set(this.data),this.texture&&a.deleteTexture(this.texture),this.data=o,this.capacity=r,this.textureWidth=i.width,this.textureHeight=i.height,this.createTexture(),this.dirty=!0,this.dirtyRangeStart=0,this.dirtyRangeEnd=this.nextIndex}}},{key:"allocate",value:function(n){var r=this.indexMap.get(n);if(r!==void 0)return r;var a;return this.freeIndices.length>0?a=this.freeIndices.pop():(a=this.nextIndex++,a>=this.capacity&&this.resize(a+1)),this.indexMap.set(n,a),a}},{key:"free",value:function(n){var r=this.indexMap.get(n);if(r!==void 0){this.indexMap.delete(n),this.freeIndices.push(r);for(var a=r*this.TEXELS_PER_ITEM*4,i=0;i<this.TEXELS_PER_ITEM*4;i++)this.data[a+i]=0;this.markDirty(r)}}},{key:"getIndex",value:function(n){var r;return(r=this.indexMap.get(n))!==null&&r!==void 0?r:-1}},{key:"has",value:function(n){return this.indexMap.has(n)}},{key:"markDirty",value:function(n){this.dirty=!0,this.dirtyRangeStart=Math.min(this.dirtyRangeStart,n),this.dirtyRangeEnd=Math.max(this.dirtyRangeEnd,n+1)}},{key:"upload",value:function(){if(!(!this.dirty||!this.texture)){var n=this.gl,r=this.textureWidth;n.activeTexture(n.TEXTURE0),n.bindTexture(n.TEXTURE_2D,this.texture);var a=this.dirtyRangeStart*this.TEXELS_PER_ITEM,i=Math.min(this.dirtyRangeEnd*this.TEXELS_PER_ITEM,this.capacity*this.TEXELS_PER_ITEM);if(a<i)for(var o=Math.floor(a/r),s=Math.floor((i-1)/r),l=o;l<=s;l++){var u=l*r,c=Math.min(u+r,this.capacity*this.TEXELS_PER_ITEM),d=Math.max(a,u),f=Math.min(i,c);if(d<f){var h=d-u,g=f-d,m=this.data.subarray(d*4,f*4);n.texSubImage2D(n.TEXTURE_2D,0,h,l,g,1,n.RGBA,n.FLOAT,m)}}n.bindTexture(n.TEXTURE_2D,null),this.dirty=!1,this.dirtyRangeStart=1/0,this.dirtyRangeEnd=-1}}},{key:"bind",value:function(n){var r=this.gl;r.activeTexture(r.TEXTURE0+n),r.bindTexture(r.TEXTURE_2D,this.texture)}},{key:"getTexture",value:function(){return this.texture}},{key:"getCapacity",value:function(){return this.capacity}},{key:"getTextureWidth",value:function(){return this.textureWidth}},{key:"getTextureHeight",value:function(){return this.textureHeight}},{key:"getTexelsPerItem",value:function(){return this.TEXELS_PER_ITEM}},{key:"getCount",value:function(){return this.indexMap.size}},{key:"getHighWaterMark",value:function(){return this.nextIndex}},{key:"isDirty",value:function(){return this.dirty}},{key:"clear",value:function(){this.indexMap.clear(),this.freeIndices=[],this.nextIndex=0,this.data.fill(0),this.dirty=!0,this.dirtyRangeStart=0,this.dirtyRangeEnd=this.capacity}},{key:"restore",value:function(){this.createTexture(),this.dirty=!1,this.dirtyRangeStart=1/0,this.dirtyRangeEnd=-1}},{key:"kill",value:function(){this.texture&&(this.gl.deleteTexture(this.texture),this.texture=null),this.indexMap.clear(),this.freeIndices=[]}}])})();function ke(e){var t={},n={},r=0,a=G(e),i;try{for(a.s();!(i=a.n()).done;){var o=i.value,s=G(o.attributes),l;try{for(s.s();!(l=s.n()).done;){var u=l.value,c=u.name.replace(/^a_/,"");c in t||(t[c]=r,n[c]=u,r+=u.size)}}catch(d){s.e(d)}finally{s.f()}}}catch(d){a.e(d)}finally{a.f()}return{floatsPerItem:r,texelsPerItem:Math.max(1,Math.ceil(r/4)),offsets:t,specs:n}}var Cr=(function(e){function t(n,r,a){var i;return K(this,t),i=ae(this,t,[n,r.texelsPerItem,a]),i.floatsPerItem=r.floatsPerItem,i}return ie(t,e),Z(t,[{key:"updateAllAttributes",value:function(r,a){var i=this.indexMap.get(r);i===void 0&&(i=this.allocate(r));for(var o=i*this.TEXELS_PER_ITEM*4,s=Math.min(a.length,this.floatsPerItem),l=0;l<s;l++)this.data[o+l]=a[l];this.markDirty(i)}},{key:"updateAllAttributesAtRow",value:function(r,a){r>=this.capacity&&this.resize(r+1);for(var i=r*this.TEXELS_PER_ITEM*4,o=Math.min(a.length,this.floatsPerItem),s=0;s<o;s++)this.data[i+s]=a[s];this.markDirty(r)}}])})(_n);function kr(e,t,n){for(var r=[],a=new Set,i=0;i<e.length;i++){var o=e[i],s=G(o.attributes),l;try{for(s.s();!(l=s.n()).done;){var u=l.value,c=u.name.replace(/^a_/,"");if(!a.has(c)){a.add(c);var d=t.offsets[c];if(d!==void 0){var f=n?.get(i);r.push({sourceKey:u.source||c,packedOffset:d,size:u.size,isColor:u.size===4&&!!u.normalized,defaultNum:typeof u.defaultValue=="number"?u.defaultValue:u.defaultValue===!0?1:0,defaultColor:typeof u.defaultValue=="string"?u.defaultValue:"",sourceIndex:i,hasLifecycleHook:!!(f!=null&&f.getAttributeData)})}}}}catch(h){s.e(h)}finally{s.f()}}return r}function Lr(e,t,n,r,a,i){n.fill(0);for(var o=0,s=e.length;o<s;o++){var l=e[o],u=l.packedOffset,c=void 0;if(l.hasLifecycleHook){var d=a.get(l.sourceIndex-i);c=d.getAttributeData(t,l.sourceKey),c===null&&(c=t[l.sourceKey])}else c=t[l.sourceKey];if(l.isColor){var f=typeof c=="string"?c:l.defaultColor||r,h=Ze(f),g=Q(h,4),m=g[0],v=g[1],p=g[2],b=g[3];n[u]=m/255,n[u+1]=v/255,n[u+2]=p/255,n[u+3]=b/255}else if(l.size===1)n[u]=typeof c=="number"?c:typeof c=="boolean"?c?1:0:l.defaultNum;else{var y=Array.isArray(c)?c:null;if(y)for(var _=0;_<l.size;_++){var x;n[u+_]=(x=y[_])!==null&&x!==void 0?x:0}}}}var vr=["r","g","b","a"];function Ot(e,t){var n=e.offsets,r=e.specs,a=e.texelsPerItem,i=e.floatsPerItem,o=Object.keys(n);if(o.length===0||i===0)return{fetchCode:"",varyingAssignments:""};var s=t.varPrefix,l=t.baseTexelExpr,u=t.textureWidthUniform,c=t.textureSamplerUniform,d=t.outputPrefix,f=d===void 0?"v_":d,h=[];h.push("  int ".concat(s,"BaseTexel = ").concat(l,";")),h.push("");for(var g=0;g<a;g++)h.push("  ivec2 ".concat(s,"Coord").concat(g," = ivec2((").concat(s,"BaseTexel + ").concat(g,") % ").concat(u,", (").concat(s,"BaseTexel + ").concat(g,") / ").concat(u,");")),h.push("  vec4 ".concat(s,"Texel").concat(g," = texelFetch(").concat(c,", ").concat(s,"Coord").concat(g,", 0);"));h.push("");for(var m=[],v=function(){var _=b[p],x=r[_],T=n[_],E=Math.floor(T/4),S=T%4,w="".concat(s,"Texel").concat(E),A="".concat(s,"Texel").concat(E+1),C="".concat(s,"Fetched_").concat(_);if(x.size===1)h.push("  float ".concat(C," = ").concat(w,".").concat(vr[S],";"));else if(x.size===4&&S===0)h.push("  vec4 ".concat(C," = ").concat(w,";"));else{var D=S+x.size;if(D<=4){var F="vec".concat(x.size),k=vr.slice(S,D).join("");h.push("  ".concat(F," ").concat(C," = ").concat(w,".").concat(k,";"))}else{var L=x.size===4?"vec4":"vec".concat(x.size),z=vr.slice(S).map(function(I){return"".concat(w,".").concat(I)}),N=vr.slice(0,D-4).map(function(I){return"".concat(A,".").concat(I)}),P=[].concat($(z),$(N)).join(", ");h.push("  ".concat(L," ").concat(C," = ").concat(L,"(").concat(P,");"))}}m.push("  ".concat(f).concat(_," = ").concat(C,";"))},p=0,b=o;p<b.length;p++)v();return{fetchCode:h.join(`
`),varyingAssignments:m.join(`
`)}}function Ga(e,t,n){if(!(!t||n.type==="sampler2D"))switch(n.type){case"float":e.uniform1f(t,n.value);break;case"int":case"bool":e.uniform1i(t,n.value);break;case"vec2":e.uniform2fv(t,n.value);break;case"vec3":e.uniform3fv(t,n.value);break;case"vec4":e.uniform4fv(t,n.value);break;case"mat3":e.uniformMatrix3fv(t,!1,n.value);break;case"mat4":e.uniformMatrix4fv(t,!1,n.value);break}}var Rh=R(R(R(R(R(R(R(R({},WebGL2RenderingContext.BOOL,1),WebGL2RenderingContext.BYTE,1),WebGL2RenderingContext.UNSIGNED_BYTE,1),WebGL2RenderingContext.SHORT,2),WebGL2RenderingContext.UNSIGNED_SHORT,2),WebGL2RenderingContext.INT,4),WebGL2RenderingContext.UNSIGNED_INT,4),WebGL2RenderingContext.FLOAT,4);function mr(e){var t=e.match(/^(#version[^\n]*\n)/);return t?t[1]+`#define PICKING_MODE
`+e.slice(t[1].length):`#define PICKING_MODE
`+e}var rt=(function(){function e(t,n,r){K(this,e),R(this,"floats",new Float32Array),R(this,"ints",new Uint32Array),R(this,"constantArray",new Float32Array),R(this,"capacity",0),R(this,"verticesCount",0),R(this,"bufferGeneration",0),R(this,"uploadedGeneration",new Map),R(this,"constantBufferGeneration",0),R(this,"uploadedConstantGeneration",new Map),R(this,"renderOffset",0),R(this,"renderCount",-1),R(this,"debugStats",{drawCalls:0,verticesDrawn:0,bufferUploadBytes:0}),R(this,"shadersLogged",!1),R(this,"pickProgram",null);var a=this.getDefinition();if(this.VERTICES=a.VERTICES,this.VERTEX_SHADER_SOURCE=a.VERTEX_SHADER_SOURCE,this.FRAGMENT_SHADER_SOURCE=a.FRAGMENT_SHADER_SOURCE,this.UNIFORMS=a.UNIFORMS,this.ATTRIBUTES=a.ATTRIBUTES,this.METHOD=a.METHOD,this.CONSTANT_ATTRIBUTES="CONSTANT_ATTRIBUTES"in a?a.CONSTANT_ATTRIBUTES:[],this.CONSTANT_DATA="CONSTANT_DATA"in a?a.CONSTANT_DATA:[],this.isInstanced="CONSTANT_ATTRIBUTES"in a,this.ATTRIBUTES_ITEMS_COUNT=xr(this.ATTRIBUTES),this.STRIDE=this.VERTICES*this.ATTRIBUTES_ITEMS_COUNT,this.renderer=r,this.normalProgram=this.getProgramInfo("normal",t,a.VERTEX_SHADER_SOURCE,a.FRAGMENT_SHADER_SOURCE,null),this.pickProgram=this.getProgramInfo("pick",t,mr(a.VERTEX_SHADER_SOURCE),mr(a.FRAGMENT_SHADER_SOURCE),null),this.isInstanced){var i=xr(this.CONSTANT_ATTRIBUTES);if(this.CONSTANT_DATA.length!==this.VERTICES)throw new Error("Program: error while getting constant data (expected ".concat(this.VERTICES," items, received ").concat(this.CONSTANT_DATA.length," instead)"));this.constantArray=new Float32Array(this.CONSTANT_DATA.length*i);for(var o=0;o<this.CONSTANT_DATA.length;o++){var s=this.CONSTANT_DATA[o];if(s.length!==i)throw new Error("Program: error while getting constant data (one vector has ".concat(s.length," items instead of ").concat(i,")"));for(var l=0;l<s.length;l++)this.constantArray[o*i+l]=s[l]}this.STRIDE=this.ATTRIBUTES_ITEMS_COUNT}}return Z(e,[{key:"kill",value:function(){ka(this.normalProgram),this.pickProgram&&ka(this.pickProgram)}},{key:"resetDebugStats",value:function(){this.debugStats.drawCalls=0,this.debugStats.verticesDrawn=0,this.debugStats.bufferUploadBytes=0}},{key:"getProgramInfo",value:function(n,r,a,i,o){var s=r.createBuffer();if(s===null)throw new Error("Program: error while creating the WebGL buffer.");var l=Er(r,a),u=Sr(r,i),c=Rr(r,[l,u]),d={};this.UNIFORMS.forEach(function(g){var m=r.getUniformLocation(c,g);m&&(d[g]=m)});var f={};this.ATTRIBUTES.forEach(function(g){f[g.name]=r.getAttribLocation(c,g.name)});var h;if(this.isInstanced&&(this.CONSTANT_ATTRIBUTES.forEach(function(g){f[g.name]=r.getAttribLocation(c,g.name)}),h=r.createBuffer(),h===null))throw new Error("Program: error while creating the WebGL constant buffer.");return{name:n,program:c,gl:r,frameBuffer:o,buffer:s,constantBuffer:h||{},uniformLocations:d,attributeLocations:f,isPicking:n==="pick",vertexShader:l,fragmentShader:u}}},{key:"bindProgram",value:function(n){var r=this,a=0,i=n.gl,o=n.buffer;if(this.isInstanced){if(i.bindBuffer(i.ARRAY_BUFFER,n.constantBuffer),a=0,this.CONSTANT_ATTRIBUTES.forEach(function(c){return a+=r.bindAttribute(c,n,a,!1)}),this.uploadedConstantGeneration.get(n.constantBuffer)!==this.constantBufferGeneration){var l;i.bufferData(i.ARRAY_BUFFER,this.constantArray,i.STATIC_DRAW),this.uploadedConstantGeneration.set(n.constantBuffer,this.constantBufferGeneration),(l=this.renderer)!==null&&l!==void 0&&l.getSetting("DEBUG_logRenderStats")&&(this.debugStats.bufferUploadBytes+=this.constantArray.byteLength)}if(i.bindBuffer(i.ARRAY_BUFFER,n.buffer),a=this.renderOffset*this.ATTRIBUTES_ITEMS_COUNT*Float32Array.BYTES_PER_ELEMENT,this.ATTRIBUTES.forEach(function(c){return a+=r.bindAttribute(c,n,a,!0)}),this.uploadedGeneration.get(o)!==this.bufferGeneration){var u;i.bufferData(i.ARRAY_BUFFER,this.floats,i.DYNAMIC_DRAW),this.uploadedGeneration.set(o,this.bufferGeneration),(u=this.renderer)!==null&&u!==void 0&&u.getSetting("DEBUG_logRenderStats")&&(this.debugStats.bufferUploadBytes+=this.floats.byteLength)}}else if(i.bindBuffer(i.ARRAY_BUFFER,o),a=0,this.ATTRIBUTES.forEach(function(c){return a+=r.bindAttribute(c,n,a)}),this.uploadedGeneration.get(o)!==this.bufferGeneration){var s;i.bufferData(i.ARRAY_BUFFER,this.floats,i.DYNAMIC_DRAW),this.uploadedGeneration.set(o,this.bufferGeneration),(s=this.renderer)!==null&&s!==void 0&&s.getSetting("DEBUG_logRenderStats")&&(this.debugStats.bufferUploadBytes+=this.floats.byteLength)}i.bindBuffer(i.ARRAY_BUFFER,null)}},{key:"unbindProgram",value:function(n){var r=this;this.isInstanced?(this.CONSTANT_ATTRIBUTES.forEach(function(a){return r.unbindAttribute(a,n,!1)}),this.ATTRIBUTES.forEach(function(a){return r.unbindAttribute(a,n,!0)})):this.ATTRIBUTES.forEach(function(a){return r.unbindAttribute(a,n)})}},{key:"bindAttribute",value:function(n,r,a,i){var o=Rh[n.type];if(typeof o!="number")throw new Error('Program.bind: yet unsupported attribute type "'.concat(n.type,'"'));var s=r.attributeLocations[n.name],l=r.gl;if(s!==-1){l.enableVertexAttribArray(s);var u=this.isInstanced?(i?this.ATTRIBUTES_ITEMS_COUNT:xr(this.CONSTANT_ATTRIBUTES))*Float32Array.BYTES_PER_ELEMENT:this.ATTRIBUTES_ITEMS_COUNT*Float32Array.BYTES_PER_ELEMENT;l.vertexAttribPointer(s,n.size,n.type,n.normalized||!1,u,a),this.isInstanced&&i&&l.vertexAttribDivisor(s,1)}return n.size*o}},{key:"unbindAttribute",value:function(n,r,a){var i=r.attributeLocations[n.name],o=r.gl;i!==-1&&(o.disableVertexAttribArray(i),this.isInstanced&&a&&o.vertexAttribDivisor(i,0))}},{key:"reallocate",value:function(n){n!==this.capacity&&(this.capacity=n,this.verticesCount=this.VERTICES*n,this.floats=new Float32Array(this.isInstanced?this.capacity*this.ATTRIBUTES_ITEMS_COUNT:this.verticesCount*this.ATTRIBUTES_ITEMS_COUNT),this.ints=new Uint32Array(this.floats.buffer),this.invalidateBuffers())}},{key:"invalidateBuffers",value:function(){this.bufferGeneration++,this.constantBufferGeneration++}},{key:"hasNothingToRender",value:function(){return this.verticesCount===0}},{key:"setTypedUniform",value:function(n,r){var a;Ga(r.gl,(a=r.uniformLocations[n.name])!==null&&a!==void 0?a:null,n)}},{key:"renderProgram",value:function(n,r){var a=r.gl,i=r.program,o=r.isPicking;o?a.disable(a.BLEND):a.enable(a.BLEND),a.useProgram(i),this.setUniforms(n,r),this.drawWebGL(this.METHOD,r)}},{key:"render",value:function(n,r,a){var i;if(!this.shadersLogged&&(i=this.renderer)!==null&&i!==void 0&&i.getSetting("DEBUG_logShaders")&&(this.shadersLogged=!0,console.log("[sigma] DEBUG_logShaders: ".concat(this.constructor.name),{normal:{vertexShaderSource:this.VERTEX_SHADER_SOURCE,fragmentShaderSource:this.FRAGMENT_SHADER_SOURCE},pick:{vertexShaderSource:mr(this.VERTEX_SHADER_SOURCE),fragmentShaderSource:mr(this.FRAGMENT_SHADER_SOURCE)}})),!this.hasNothingToRender()){this.renderOffset=r??0,this.renderCount=a??-1;var o=this.normalProgram.gl;if(o.bindFramebuffer(o.FRAMEBUFFER,null),o.viewport(0,0,n.width*n.pixelRatio,n.height*n.pixelRatio),this.bindProgram(this.normalProgram),this.renderProgram(n,this.normalProgram),this.unbindProgram(this.normalProgram),this.pickProgram&&n.pickingFrameBuffer){var s=Math.ceil(n.width*n.pixelRatio/n.downSizingRatio),l=Math.ceil(n.height*n.pixelRatio/n.downSizingRatio);o.bindFramebuffer(o.FRAMEBUFFER,n.pickingFrameBuffer),o.viewport(0,0,s,l),this.bindProgram(this.pickProgram),this.renderProgram(n,this.pickProgram),this.unbindProgram(this.pickProgram),o.bindFramebuffer(o.FRAMEBUFFER,null),o.viewport(0,0,n.width*n.pixelRatio,n.height*n.pixelRatio)}}}},{key:"drawWebGL",value:function(n,r){var a,i=r.gl,o=this.renderCount>=0?this.renderCount:this.capacity;(a=this.renderer)!==null&&a!==void 0&&a.getSetting("DEBUG_logRenderStats")&&(this.debugStats.drawCalls++,this.debugStats.verticesDrawn+=o*this.VERTICES),this.isInstanced?i.drawArraysInstanced(n,0,this.VERTICES,o):i.drawArrays(n,this.renderOffset*this.VERTICES,o*this.VERTICES)}}])})(),at=2,xn=7,rs={below:0,above:1,left:2,right:3};function Ah(){var e=`#version 300 es
precision highp float;

uniform mat3 u_matrix;
uniform vec2 u_resolution;
uniform float u_labelPixelSnapping;
uniform float u_sizeRatio;
uniform float u_correctionRatio;
uniform float u_cameraAngle;
uniform float u_pixelRatio;
uniform float u_labelMargin;
uniform float u_zoomLabelSizeRatio;

uniform sampler2D u_nodeDataTexture;
uniform int u_nodeDataTextureWidth;
uniform sampler2D u_nodeFrameTexture;
uniform int u_nodeFrameTextureWidth;

// Atlas size is fixed at 2048\xD72048 \u2014 matches AttachmentManager.ATLAS_SIZE.
uniform sampler2D u_atlasTexture;
const vec2 u_atlasSize = vec2(2048.0);

// Per-instance
in float a_nodeIndex;           // node-data texture index
in vec4 a_atlasRect;            // x, y, width, height in atlas pixels
in vec2 a_attachmentSize;       // attachment dimensions (CSS px)
in float a_positionMode;        // label position: 0=right 1=left 2=above 3=below 4=over
in float a_attachmentPlacement; // 0=below 1=above 2=left 3=right (relative to label)
in float a_labelWidth;          // label width (CSS px)
in float a_labelHeight;         // label height: font line box (CSS px)
in float a_textHeight;          // actual glyph height (CSS px)
in float a_labelAngle;          // label rotation angle (radians)

// Per-vertex (constant)
in vec2 a_quadCorner;           // [-1,-1], [1,-1], [-1,1], [1,1]

out vec2 v_texCoord;

`.concat(Be,`
`).concat(nt,`
`).concat(yt,`
`).concat(Ma,`
`).concat(Yo,`

void main() {
  int nodeIdx = int(a_nodeIndex);

  // Node data: (x, y, size, shapeId).
  vec4 nodeData = readNodeData(u_nodeDataTexture, u_nodeDataTextureWidth, nodeIdx);
  vec2 nodePos = nodeData.xy;
  float nodeSize = nodeData.z;

  // Normalized edge distance from the shared frame texture (the frame-pass ran
  // the SDF search once; we just read the result).
  float edgeDist = readFrameTexel(u_nodeFrameTexture, u_nodeFrameTextureWidth, nodeIdx).r;

  // Per-node label rotation alignment: 0 = viewport, 1 = label turns with camera.
  float labelRotation = readNodeFlags(u_nodeDataTexture, u_nodeDataTextureWidth, nodeIdx).g;

  vec3 nodeClip = u_matrix * vec3(nodePos, 1.0);

  // Node radius in physical pixels (matches label/background).
  float matrixScaleX = length(vec2(u_matrix[0][0], u_matrix[1][0]));
  float nodeRadiusGraphSpace = nodeSize * u_correctionRatio / u_sizeRatio * 2.0;
  float nodeRadiusPixels = nodeRadiusGraphSpace * matrixScaleX * u_resolution.x / 2.0;

  // CSS-px inputs -> physical px, scaled by the zoom-dependent label ratio, so
  // the attachment stays glued to the label box exactly as the label scales.
  float zoomScale = u_zoomLabelSizeRatio;
  vec2 labelHalf = vec2(a_labelWidth, a_labelHeight) * 0.5 * zoomScale * u_pixelRatio;
  vec2 attachHalf = a_attachmentSize * 0.5 * zoomScale * u_pixelRatio;
  float gap = `).concat(at.toFixed(1),` * zoomScale * u_pixelRatio;
  float labelMargin = u_labelMargin * zoomScale * u_pixelRatio;

  // Graph-aligned labels add the camera angle so the attachment orbits with the box.
  mat2 labelRotMat = rotate2D(a_labelAngle - labelRotation * u_cameraAngle);

  // Label box center relative to the node center (pre-rotation). The shape-aware
  // edge distance comes from the frame texture, so placement matches the label.
  vec2 boxCenter = vec2(0.0);
  if (a_positionMode < 4.0) {
    float labelStart = nodeRadiusPixels * edgeDist + labelMargin;
    float textHalf = a_textHeight * 0.5 * zoomScale * u_pixelRatio;
    boxCenter = labelBoxCenter(a_positionMode, labelStart, labelHalf, textHalf);
  }

  // Horizontal anchor of below/above attachments tracks the label position, so
  // the attachment hangs from the label edge nearest the node (or is centered
  // when the label itself is node-centered): right->left, left->right, else center.
  float anchorX = 0.0;
  if (a_positionMode < 0.5) anchorX = -labelHalf.x + attachHalf.x;       // right label
  else if (a_positionMode < 1.5) anchorX = labelHalf.x - attachHalf.x;   // left label

  // Attachment center relative to the box (pre-rotation, Y-down). This offset
  // depends only on the box, the gap and the attachment size \u2014 never the shape.
  vec2 attachCenter;
  if (a_attachmentPlacement < 0.5) {
    // below
    attachCenter = boxCenter + vec2(anchorX, labelHalf.y + gap + attachHalf.y);
  } else if (a_attachmentPlacement < 1.5) {
    // above
    attachCenter = boxCenter + vec2(anchorX, -(labelHalf.y + gap + attachHalf.y));
  } else if (a_attachmentPlacement < 2.5) {
    // left, top-aligned
    attachCenter = boxCenter + vec2(-(labelHalf.x + gap + attachHalf.x), -labelHalf.y + attachHalf.y);
  } else {
    // right, top-aligned
    attachCenter = boxCenter + vec2(labelHalf.x + gap + attachHalf.x, -labelHalf.y + attachHalf.y);
  }

  // Rotate the whole assembly by the label angle, around the node center.
  vec2 rotatedCenter = labelRotMat * attachCenter;
  vec2 cornerOffset = labelRotMat * (a_quadCorner * attachHalf);

  // Node center in screen px (Y-down).
  vec2 nodeScreen = vec2(
    (nodeClip.x + 1.0) * u_resolution.x,
    (1.0 - nodeClip.y) * u_resolution.y
  ) * 0.5;

  // Snap node center to the pixel grid so label/backdrop/attachment move as one.
  vec2 snapDelta = (round(nodeScreen) - nodeScreen) * u_labelPixelSnapping;

  // Snap the quad's top-left to integer pixels so atlas texels map 1:1.
  vec2 centerScreen = nodeScreen + rotatedCenter + snapDelta;
  vec2 topLeft = centerScreen - attachHalf;
  topLeft = mix(topLeft, round(topLeft), u_labelPixelSnapping);
  centerScreen = topLeft + attachHalf;

  vec2 vertexScreen = centerScreen + cornerOffset;
  gl_Position = vec4(
    vertexScreen.x * 2.0 / u_resolution.x - 1.0,
    1.0 - vertexScreen.y * 2.0 / u_resolution.y,
    0.0, 1.0
  );

  vec2 texOrigin = a_atlasRect.xy / u_atlasSize;
  vec2 texSize = a_atlasRect.zw / u_atlasSize;
  vec2 uv = (a_quadCorner + 1.0) / 2.0;
  v_texCoord = texOrigin + uv * texSize;
}
`);return e}var wh=`#version 300 es
precision highp float;

uniform sampler2D u_atlasTexture;

in vec2 v_texCoord;

layout(location = 0) out vec4 fragColor;
#ifdef PICKING_MODE
layout(location = 1) out vec4 pickColor;
#endif

void main() {
  // Canvas textures are premultiplied; output directly for (ONE, 1-SRC_ALPHA) blending
  vec4 color = texture(u_atlasTexture, v_texCoord);
  if (color.a < 0.01) discard;
  fragColor = color;
#ifdef PICKING_MODE
  pickColor = vec4(0.0); // Attachments are not pickable
#endif
}
`;function Dh(e,t,n,r){var a,i,o=r.label,s=o===void 0?{}:o,l=(a=s.margin)!==null&&a!==void 0?a:bt,u=(i=s.zoomToLabelSizeRatioFunction)!==null&&i!==void 0?i:function(){return 1},c=Ah(),d=wh,f=(function(h){function g(){var m;K(this,g);for(var v=arguments.length,p=new Array(v),b=0;b<v;b++)p[b]=arguments[b];return m=ae(this,g,[].concat(p)),R(m,"totalCount",0),R(m,"bufferCapacity",0),m}return ie(g,h),Z(g,[{key:"getDefinition",value:function(){var v=WebGL2RenderingContext,p=v.FLOAT,b=v.TRIANGLE_STRIP;return{VERTICES:4,VERTEX_SHADER_SOURCE:c,FRAGMENT_SHADER_SOURCE:d,METHOD:b,UNIFORMS:["u_matrix","u_resolution","u_labelPixelSnapping","u_sizeRatio","u_correctionRatio","u_cameraAngle","u_pixelRatio","u_labelMargin","u_zoomLabelSizeRatio","u_nodeDataTexture","u_nodeDataTextureWidth","u_nodeFrameTexture","u_nodeFrameTextureWidth","u_atlasTexture"],ATTRIBUTES:[{name:"a_nodeIndex",size:1,type:p},{name:"a_atlasRect",size:4,type:p},{name:"a_attachmentSize",size:2,type:p},{name:"a_positionMode",size:1,type:p},{name:"a_attachmentPlacement",size:1,type:p},{name:"a_labelWidth",size:1,type:p},{name:"a_labelHeight",size:1,type:p},{name:"a_textHeight",size:1,type:p},{name:"a_labelAngle",size:1,type:p}],CONSTANT_ATTRIBUTES:[{name:"a_quadCorner",size:2,type:p}],CONSTANT_DATA:[[-1,-1],[1,-1],[-1,1],[1,1]]}}},{key:"processAttachment",value:function(v,p){var b=this.floats,y=v*this.STRIDE;b[y++]=p.nodeIndex,b[y++]=p.atlasX,b[y++]=p.atlasY,b[y++]=p.atlasW,b[y++]=p.atlasH,b[y++]=p.attachWidth,b[y++]=p.attachHeight,b[y++]=p.positionMode,b[y++]=p.attachmentPlacement,b[y++]=p.labelWidth,b[y++]=p.labelHeight,b[y++]=p.textHeight,b[y++]=p.labelAngle}},{key:"setUniforms",value:function(v,p){var b=p.gl,y=p.uniformLocations;b.uniformMatrix3fv(y.u_matrix,!1,v.matrix),b.uniform2f(y.u_resolution,v.width*v.pixelRatio,v.height*v.pixelRatio),b.uniform1f(y.u_labelPixelSnapping,v.labelPixelSnapping),b.uniform1f(y.u_sizeRatio,v.sizeRatio),b.uniform1f(y.u_correctionRatio,v.correctionRatio),b.uniform1f(y.u_cameraAngle,v.cameraAngle),b.uniform1f(y.u_pixelRatio,v.pixelRatio),b.uniform1f(y.u_labelMargin,g.labelMargin),b.uniform1f(y.u_zoomLabelSizeRatio,1/u(v.zoomRatio)),b.uniform1i(y.u_nodeDataTexture,v.nodeDataTextureUnit),b.uniform1i(y.u_nodeDataTextureWidth,v.nodeDataTextureWidth),b.uniform1i(y.u_nodeFrameTexture,v.nodeFrameTextureUnit),b.uniform1i(y.u_nodeFrameTextureWidth,v.nodeFrameTextureWidth),b.uniform1i(y.u_atlasTexture,xn)}},{key:"reallocateAttachments",value:function(v){this.totalCount=v,v>this.bufferCapacity&&(this.bufferCapacity=Math.max(v,Math.ceil(this.bufferCapacity*1.5)||10),de(g,"reallocate",this,3)([this.bufferCapacity]))}},{key:"hasNothingToRender",value:function(){return this.totalCount===0}},{key:"drawWebGL",value:function(v,p){var b=p.gl;this.totalCount!==0&&b.drawArraysInstanced(v,0,this.VERTICES,this.totalCount)}}])})(rt);return R(f,"labelMargin",l),new f(e,t,n)}var et=5;function Fr(e,t){return[].concat($(t),$(e.map(function(n){var r;return{attributes:(r=n.attributes)!==null&&r!==void 0?r:[]}})))}function jt(e,t){var n=ke(Fr(e,t)),r=new Set(e.flatMap(function(u){var c;return(c=u.attributes)!==null&&c!==void 0?c:[]}).map(function(u){return u.name.replace(/^a_/,"")})),a={},i={},o=G(r),s;try{for(o.s();!(s=o.n()).done;){var l=s.value;a[l]=n.offsets[l],i[l]=n.specs[l]}}catch(u){o.e(u)}finally{o.f()}return B(B({},n),{},{offsets:a,specs:i})}function as(e,t){var n=new Set(["u_matrix","u_sizeRatio","u_correctionRatio","u_cameraAngle","u_pickingPadding","u_nodeDataTexture","u_layerAttributeTexture"]),r=new Set,a=e.flatMap(function(h){return h.uniforms}).filter(function(h){return n.has(h.name)||r.has(h.name)?!1:(r.add(h.name),!0)}).map(function(h){return"uniform ".concat(h.type," ").concat(h.name,";")}).join(`
`),i=t.flatMap(function(h){return h.uniforms}).filter(function(h){return n.has(h.name)||r.has(h.name)?!1:(r.add(h.name),!0)}).map(function(h){return"uniform ".concat(h.type," ").concat(h.name,";")}).join(`
`),o=Fr(e,t),s=new Set,l=o.flatMap(function(h){return h.attributes}).filter(function(h){var g=h.name.replace(/^a_/,"");return s.has(g)?!1:(s.add(g),!0)}).map(function(h){var g=h.name.replace(/^a_/,""),m=h.size===1?"float":"vec".concat(h.size);return"out ".concat(m," v_").concat(g,";")}).join(`
`),u=Ot(ke(o),{varPrefix:"layer",baseTexelExpr:"nodeIdx * u_layerAttributeTexelsPerNode",textureWidthUniform:"u_layerAttributeTextureWidth",textureSamplerUniform:"u_layerAttributeTexture"}),c=u.fetchCode,d=u.varyingAssignments,f=`#version 300 es

// Standard node attributes (per instance) - minimal buffer usage
in float a_nodeIndex;  // Index into node data texture AND layer attribute texture
in vec4 a_id;          // Node ID for picking
in float a_opacity;    // Node opacity, applied once to the final fragment

// Constant attributes (per vertex, same for all instances)
in vec2 a_quadCorner;  // (-1,-1), (1,-1), (1,1), (-1,1) for quad corners

// Standard uniforms
uniform mat3 u_matrix;
uniform float u_sizeRatio;
uniform float u_correctionRatio;
uniform float u_cameraAngle;
#ifdef PICKING_MODE
uniform float u_pickingPadding;
#endif
uniform sampler2D u_nodeDataTexture;
uniform int u_nodeDataTextureWidth;

// Layer attribute texture uniforms
uniform sampler2D u_layerAttributeTexture;
uniform int u_layerAttributeTextureWidth;
uniform int u_layerAttributeTexelsPerNode;

`.concat(a,`
`).concat(i,`

// Standard varyings
out vec2 v_uv;                    // Normalized coordinates [-1, 1]
out vec4 v_id;
out float v_opacity;
out float v_antialiasingWidth;    // Width for antialiasing in UV space
out float v_pixelSize;            // Node size in pixels (for pixel-mode borders)
out float v_pixelToUV;            // Conversion factor: multiply by this to convert screen pixels to UV units
out float v_shapeId;              // Shape ID for multi-shape programs

// Layer varyings
`).concat(l,`

// Node-data fetch helpers (geometry texel + rotation-flags texel)
`).concat(Be,`
`).concat(nt,`

void main() {
  // Fetch node geometry: vec4(x, y, size, shapeId).
  int nodeIdx = int(a_nodeIndex);

  // Hidden nodes are flagged with a negative row by NodeProgram.process(). Push
  // the whole quad outside the clip volume: every vertex lands at the same
  // out-of-range position, so the primitive is fully clipped and rasterizes
  // nothing \u2014 neither to the frame buffer nor to the picking buffer.
  if (nodeIdx < 0) {
    gl_Position = vec4(2.0, 0.0, 0.0, 1.0);
    v_id = vec4(0.0);
    return;
  }

  vec4 nodeData = readNodeData(u_nodeDataTexture, u_nodeDataTextureWidth, nodeIdx);
  vec2 a_position = nodeData.xy;
  float a_size = nodeData.z;
  v_shapeId = nodeData.w;  // Pass shape ID to fragment shader

  // Per-node rotation alignment: 0 = viewport (screen-upright), 1 = graph.
  float nodeRotation = readNodeFlags(u_nodeDataTexture, u_nodeDataTextureWidth, nodeIdx).r;

`).concat(c,`

  // Calculate the actual size in pixels
  float size = a_size * u_correctionRatio / u_sizeRatio * 2.0;

  // In PICKING_MODE, inflate the quad by nodePickingPadding pixels on each side
  #ifdef PICKING_MODE
    float paddedSize = size + u_pickingPadding * u_correctionRatio;
    vec2 offset = a_quadCorner * paddedSize;
  #else
    vec2 offset = a_quadCorner * size;
  #endif
  // Counter-rotate the quad offset so viewport-aligned nodes stay upright as the
  // camera turns. Graph-aligned nodes (nodeRotation=1) skip it and turn with the
  // camera. The angle scales by (1 - nodeRotation) so both fall out of one path.
  {
    float ca = u_cameraAngle * (1.0 - nodeRotation);
    float c = cos(ca);
    float s = sin(ca);
    offset = mat2(c, s, -s, c) * offset;
  }
  vec2 position = a_position + offset;

  gl_Position = vec4(
    (u_matrix * vec3(position, 1)).xy,
    0,
    1
  );

  // In PICKING_MODE, UV is scaled beyond [-1, 1] to match the inflated quad
  #ifdef PICKING_MODE
    v_uv = a_quadCorner * (paddedSize / size);
  #else
    v_uv = a_quadCorner;
  #endif

  // Pass ID to fragment shader
  v_id = a_id;
  v_opacity = a_opacity;

  // Pass pixel size for layers that need pixel-mode calculations
  // Multiply by 2 because 'size' is half-width (offset from center), not full diameter
  v_pixelSize = size * 2.0;

  // Conversion factor from screen pixels to UV units
  // Same derivation as v_antialiasingWidth which represents ~1 pixel in UV space
  // P pixels in UV space = P * u_correctionRatio / size
  v_pixelToUV = u_correctionRatio / size;

  // We use an antialiasing width of 1px (so v_pixelToUV)
  v_antialiasingWidth = v_pixelToUV;

  // Pass layer attributes to fragment shader (fetched from texture)
`).concat(d,`
}
`);return f}function is(e,t,n){var r=arguments.length>3&&arguments[3]!==void 0?arguments[3]:!0,a=t.map(function(g,m){var v="layer_".concat(g.name),p=[].concat($(g.attributes.map(function(b){return"v_".concat(b.name.replace(/^a_/,""))})),$(g.uniforms.map(function(b){return b.name}))).join(", ");return"  // Layer ".concat(m+1,": ").concat(g.name,`
  color = blendOver(color, `).concat(v,"(").concat(p,"));")}).join(`

`),i=new Set(["u_correctionRatio"]),o=new Set,s=e.flatMap(function(g){return g.uniforms}).filter(function(g){return i.has(g.name)||o.has(g.name)?!1:(o.add(g.name),!0)}).map(function(g){return"uniform ".concat(g.type," ").concat(g.name,";")}).join(`
`),l=t.flatMap(function(g){return g.uniforms}).filter(function(g){return i.has(g.name)||o.has(g.name)?!1:(o.add(g.name),!0)}).map(function(g){return"uniform ".concat(g.type," ").concat(g.name,";")}).join(`
`),u=new Set,c=Fr(e,t).flatMap(function(g){return g.attributes}).filter(function(g){var m=g.name.replace(/^a_/,"");return u.has(m)?!1:(u.add(m),!0)}).map(function(g){var m=g.name.replace(/^a_/,""),v=g.size===1?"float":"vec".concat(g.size);return"in ".concat(v," v_").concat(m,";")}).join(`
`),d=Ar(e),f=Xo(e,n),h=`#version 300 es
precision highp float;

// Standard varyings
in vec2 v_uv;
in vec4 v_id;
in float v_opacity;
in float v_antialiasingWidth;
in float v_pixelSize;
in float v_pixelToUV;
in float v_shapeId;  // Shape ID for multi-shape programs

// Standard uniforms (needed for some layer calculations like pixel-mode borders)
uniform float u_correctionRatio;
#ifdef PICKING_MODE
uniform float u_pickingPadding;
#endif

// Shape uniforms
`.concat(s,`

// Layer uniforms
`).concat(l,`

// Layer varyings
`).concat(c,`

// Fragment output (single target - picking handled via separate pass)
out vec4 fragColor;

// LayerContext struct - provides rendering context to all layers
struct LayerContext {
  float sdf;             // Signed distance from shape boundary (negative inside)
  vec2 uv;               // UV coordinates [-1, 1], center at (0,0)
  float shapeSize;       // Effective shape size (~diameter) in UV space (1.0 - aaWidth)
  float shapeHalfSize;   // Effective shape half size (~radius) in UV space
  float pixelSize;       // Node full size (~diameter) in screen pixels
  float aaWidth;         // Anti-aliasing width for smooth transitions
  float correctionRatio; // Scaling factor for consistent rendering across zoom levels
  float pixelToUV;       // Conversion factor: multiply screen pixels by this to get UV units
  float inradiusFactor;  // Ratio of inradius to circumradius (shape depth factor)
};

LayerContext context;  // Global instance, populated before layer calls

// Alpha "over" compositing for layer blending
vec4 blendOver(vec4 bg, vec4 fg) {
  float a = fg.a;
  return vec4(mix(bg.rgb, fg.rgb, a), bg.a + a * (1.0 - bg.a));
}

// SDF shape functions
`).concat(d,`

// Shape selector function (sets context.sdf and context.inradiusFactor)
`).concat(f,`

// Layer functions
`).concat(t.map(function(g){return g.glsl}).join(`

`),`

void main() {
  // 1. Setup LayerContext (available to all layer functions)
  context.shapeSize = 1.0 - v_antialiasingWidth;
  context.shapeHalfSize = context.shapeSize * 0.5;
  context.pixelSize = v_pixelSize;
  context.uv = v_uv;
  context.aaWidth = v_antialiasingWidth;
  context.correctionRatio = u_correctionRatio;
  context.pixelToUV = v_pixelToUV;

  // Query shape SDF based on shapeId (sets context.sdf and context.inradiusFactor)
  queryNodeSDF(int(v_shapeId), v_uv, context.shapeSize);

  // 2. Early discard for pixels fully outside the shape (with AA margin)
  // In PICKING_MODE, allow extra fragments up to the picking padding distance
  #ifdef PICKING_MODE
    if (context.sdf > u_pickingPadding * v_pixelToUV + context.aaWidth) discard;
  #else
    if (context.sdf > context.aaWidth) discard;
  #endif

  // 3. Apply layers sequentially with "over" compositing
  vec4 color = vec4(0.0);

`).concat(a,`

  #ifdef PICKING_MODE
    // Picking pass: output node ID for pixels within the picking area.
    if (context.sdf > u_pickingPadding * v_pixelToUV) discard;
    fragColor = v_id;
  #else
`).concat(r?`    // Visual pass: apply antialiasing at shape boundary, node opacity once
    // smoothstep provides smooth transition from opaque to transparent
    float alpha = smoothstep(context.aaWidth, -context.aaWidth, context.sdf) * v_opacity;`:`    // Visual pass: hard-edged (no anti-aliasing gradient) shape boundary, node opacity applied once
    float alpha = (context.sdf < 0.0 ? 1.0 : 0.0) * v_opacity;`,`
    // Mix with transparent to fade both color AND alpha together (avoids bright halo)
    fragColor = mix(vec4(0.0), color, alpha);
  #endif
}
`);return h}function os(e,t){var n=new Set;return n.add("u_matrix"),n.add("u_sizeRatio"),n.add("u_correctionRatio"),n.add("u_cameraAngle"),n.add("u_pickingPadding"),n.add("u_nodeDataTexture"),n.add("u_nodeDataTextureWidth"),n.add("u_layerAttributeTexture"),n.add("u_layerAttributeTextureWidth"),n.add("u_layerAttributeTexelsPerNode"),e.forEach(function(r){r.uniforms.forEach(function(a){return n.add(a.name)})}),t.forEach(function(r){r.uniforms.forEach(function(a){return n.add(a.name)})}),Array.from(n)}function ss(e){var t=WebGL2RenderingContext,n=t.UNSIGNED_BYTE,r=t.FLOAT;return[{name:"a_nodeIndex",size:1,type:r},{name:"a_id",size:4,type:n,normalized:!0},{name:"a_opacity",size:1,type:r}]}function Fa(e){var t=e.shapes,n=e.layers,r=e.shapeGlobalIds,a=e.antialias,i=a===void 0?!0:a;return{vertexShader:as(t,n),fragmentShader:is(t,n,r,i),uniforms:os(t,n),attributes:ss()}}function ls(e,t,n){var r=jt(e,t),a=r.specs;return Object.keys(a).map(function(i){return"flat ".concat(n," ").concat(a[i].size===1?"float":"vec".concat(a[i].size)," v_").concat(i,";")}).join(`
`)}function us(e){var t,n,r=e.shapes,a=e.layers,i=e.shapeGlobalIds,o=Ot(jt(r,a),{varPrefix:"nodeAttr",baseTexelExpr:"nodeIdx * u_layerAttributeTexelsPerNode",textureWidthUniform:"u_layerAttributeTextureWidth",textureSamplerUniform:"u_layerAttributeTexture"}),s=r.length===1?"float inradiusFactor = ".concat(re((t=r[0].inradiusFactor)!==null&&t!==void 0?t:1),";"):"float inradiusFactor = ".concat(re((n=r[0].inradiusFactor)!==null&&n!==void 0?n:1),`;
  switch (int(shapeId)) {
`).concat(r.map(function(u,c){var d,f=i?i[c]:c;return"    case ".concat(f,": inradiusFactor = ").concat(re((d=u.inradiusFactor)!==null&&d!==void 0?d:1),"; break;")}).join(`
`),`
    default: break;
  }`),l=`#version 300 es

in float a_nodeIndex;
in float a_labelWidth;
in float a_labelHeight;
in float a_textHeight;
in float a_positionMode;
in float a_labelAngle;
in vec4 a_backdropColor;
in vec4 a_backdropShadowColor;
in float a_backdropShadowBlur;
in float a_backdropPadding;
in vec4 a_backdropBorderColor;
in vec4 a_backdropExtra; // [borderWidth, cornerRadius, labelPadding, area]
in vec2 a_labelBoxOffset;
in vec2 a_quadCorner;

uniform mat3 u_matrix;
uniform float u_sizeRatio;
uniform float u_correctionRatio;
uniform float u_cameraAngle;
uniform vec2 u_resolution;
uniform float u_pixelRatio;
uniform float u_labelMargin;
uniform float u_zoomLabelSizeRatio;
uniform float u_labelPixelSnapping;
uniform sampler2D u_nodeDataTexture;
uniform int u_nodeDataTextureWidth;
uniform sampler2D u_nodeFrameTexture;
uniform int u_nodeFrameTextureWidth;
uniform sampler2D u_layerAttributeTexture;
uniform int u_layerAttributeTextureWidth;
uniform int u_layerAttributeTexelsPerNode;

out vec2 v_uv;
out vec2 v_nodeCenter;
out float v_nodeRadius;
out vec2 v_labelCenter;
out vec2 v_labelHalfSize;
out float v_aaWidth;
out float v_shapeId;
out float v_nodeRotation;
out float v_labelAngle;
out vec4 v_backdropColor;
out vec4 v_backdropShadowColor;
out float v_backdropShadowBlur;
out float v_backdropPadding;
out vec4 v_backdropBorderColor;
out float v_backdropBorderWidth;
out float v_backdropCornerRadius;
out float v_backdropArea;
`.concat(ls(r,a,"out"),`

`).concat(Be,`
`).concat(nt,`
`).concat(yt,`

void main() {
  int nodeIdx = int(a_nodeIndex);
`).concat(o.fetchCode,`
`).concat(o.varyingAssignments,`

  // Node data: (x, y, size, shapeId). shapeId (global) is forwarded to the
  // fragment shader, which keeps the shape SDF for the outline.
  vec4 nodeData = readNodeData(u_nodeDataTexture, u_nodeDataTextureWidth, nodeIdx);
  vec2 nodePosition = nodeData.xy;
  float nodeSize = nodeData.z;
  float shapeId = nodeData.w;

  // Per-node rotation alignment (0 = viewport, 1 = graph). nodeRotation drives
  // the fragment's shape outline; labelRotation turns the label box with camera.
  vec4 nodeFlags = readNodeFlags(u_nodeDataTexture, u_nodeDataTextureWidth, nodeIdx);
  float nodeRotation = nodeFlags.r;
  float labelRotation = nodeFlags.g;
  v_nodeRotation = nodeRotation;

  `).concat(Na,`
  // CSS pixel attributes are multiplied by u_pixelRatio to match nodeRadiusPixels,
  // which is already in physical pixels (nodeRadiusNDC * u_resolution.x / 2.0).
  float padding = a_backdropPadding * u_pixelRatio;
  float shadowBlur = a_backdropShadowBlur * u_pixelRatio;
  // Unpack a_backdropExtra: [borderWidth, cornerRadius, labelPadding, area]
  float borderWidth = a_backdropExtra.x * u_pixelRatio;
  float cornerRadius = a_backdropExtra.y * u_pixelRatio;
  float labelPad = a_backdropExtra.z * u_pixelRatio;
  float backdropArea = a_backdropExtra.w;
  // Use 2x shadowBlur so the Gaussian fully decays before the quad edge
  float totalExpansion = shadowBlur * 2.0 + borderWidth;
  float enlargedRadius = nodeRadiusPixels + padding;

  // Circumscribed radius for the quad bounds. Non-circular shapes reach past
  // their inradius out to their circumradius (= enlargedRadius / inradiusFactor),
  // in any direction once rotated. The axis-aligned quad must contain that, or
  // the fill/shadow gets clipped (square corners, triangle tip, etc.).
  `).concat(s,`
  float boundRadius = enlargedRadius / inradiusFactor;

  // Apply zoom-dependent label size scaling
  float zoomScale = u_zoomLabelSizeRatio;
  float labelW = a_labelWidth * zoomScale * u_pixelRatio;
  float labelH = a_labelHeight * zoomScale * u_pixelRatio;
  float labelMargin = u_labelMargin * zoomScale * u_pixelRatio;

  // Only apply labelPad when a label is actually present
  float effectiveLabelPad = labelW > 0.0 ? labelPad : 0.0;
  vec2 labelHalfSize = vec2(labelW * 0.5 + effectiveLabelPad, labelH * 0.5 + effectiveLabelPad);
  vec2 labelOffset = vec2(0.0);

  // Effective label angle: intrinsic angle plus the camera angle when the label
  // is graph-aligned, so the box orbits the node in lockstep with the frame-pass.
  float labelAngle = a_labelAngle - labelRotation * u_cameraAngle;
  float la_c = cos(labelAngle);
  float la_s = sin(labelAngle);
  mat2 labelRotMat = mat2(la_c, -la_s, la_s, la_c);

  vec3 nodeClip = u_matrix * vec3(nodePosition, 1.0);
  vec2 snapDelta = vec2(0.0);

  if (labelW > 0.0) {
    // labelW > 0.0 means this node's label is displayed, so the frame-pass wrote
    // its edge distance this frame.
    float edgeDistPixels = nodeRadiusPixels * readFrameTexel(u_nodeFrameTexture, u_nodeFrameTextureWidth, nodeIdx).r;
    // labelMargin matches the label shader's margin (gap from node edge to text)
    float labelStart = edgeDistPixels + labelMargin;
    // Above/below center on the actual glyph height, not the font line box, so
    // the box stays aligned with the rendered text (matches labelBoxCenter()).
    float textHalf = a_textHeight * zoomScale * u_pixelRatio * 0.5;

    // Snap node center to pixel grid so label/backdrop/attachment move as a unit
    vec2 nodeScreen = vec2(
      (nodeClip.x + 1.0) * u_resolution.x,
      (1.0 - nodeClip.y) * u_resolution.y
    ) * 0.5;
    snapDelta = (round(nodeScreen) - nodeScreen) * u_labelPixelSnapping;

    if (a_positionMode < 0.5) {
      // Right: box spans from node center to text end + padding
      float boxRightEdge = labelStart + labelW + labelPad;
      labelOffset = vec2(boxRightEdge * 0.5, 0.0);
      labelHalfSize.x = boxRightEdge * 0.5;
    } else if (a_positionMode < 1.5) {
      // Left: mirror of right
      float boxLeftEdge = labelStart + labelW + labelPad;
      labelOffset = vec2(-boxLeftEdge * 0.5, 0.0);
      labelHalfSize.x = boxLeftEdge * 0.5;
    } else if (a_positionMode < 2.5) {
      // Above: text bottom at labelStart, centered horizontally
      labelOffset = vec2(0.0, -(labelStart + textHalf));
    } else if (a_positionMode < 3.5) {
      // Below: text top at labelStart, centered horizontally
      labelOffset = vec2(0.0, labelStart + textHalf);
    }
    // over (>=4): labelOffset stays (0,0) \u2014 text centered on the node.

    // The attachment-cover shift is in label space (like the attachment itself),
    // so rotate it together with the position offset \u2014 otherwise the box drifts
    // off the attachment as the label angle grows.
    labelOffset = labelRotMat * (labelOffset + a_labelBoxOffset * zoomScale * u_pixelRatio);
  }

  // For node-only mode, zero out label dimensions
  if (backdropArea > 0.5 && backdropArea < 1.5) {
    labelHalfSize = vec2(0.0);
    labelOffset = vec2(0.0);
  }

  vec2 minBound, maxBound;

  bool hasLabelBounds = labelW > 0.0 && (backdropArea > 1.5 || backdropArea < 0.5);

  if (hasLabelBounds) {
    // Union with label bounds (area=both) or label-only bounds (area=label)
    vec2 labelMin, labelMax;

    if (a_labelAngle != 0.0) {
      vec2 corner1 = labelOffset + labelRotMat * vec2(-labelHalfSize.x, -labelHalfSize.y);
      vec2 corner2 = labelOffset + labelRotMat * vec2(labelHalfSize.x, -labelHalfSize.y);
      vec2 corner3 = labelOffset + labelRotMat * vec2(labelHalfSize.x, labelHalfSize.y);
      vec2 corner4 = labelOffset + labelRotMat * vec2(-labelHalfSize.x, labelHalfSize.y);
      labelMin = min(min(corner1, corner2), min(corner3, corner4));
      labelMax = max(max(corner1, corner2), max(corner3, corner4));
    } else {
      labelMin = labelOffset - labelHalfSize;
      labelMax = labelOffset + labelHalfSize;
    }

    if (backdropArea > 1.5) {
      // Label-only: bounds from label rect only
      minBound = labelMin - totalExpansion;
      maxBound = labelMax + totalExpansion;
    } else {
      // Both: union of node + label
      minBound = min(-vec2(boundRadius), labelMin) - totalExpansion;
      maxBound = max(vec2(boundRadius), labelMax) + totalExpansion;
    }
  } else {
    // Node-only or no visible label
    float totalRadius = boundRadius + totalExpansion;
    minBound = -vec2(totalRadius);
    maxBound = vec2(totalRadius);
  }

  vec2 quadSize = maxBound - minBound;
  vec2 quadCenter = (minBound + maxBound) * 0.5;

  vec2 localPos = quadCenter + a_quadCorner * quadSize * 0.5 + snapDelta;
  vec2 ndcOffset = localPos * 2.0 / u_resolution;
  ndcOffset.y = -ndcOffset.y;

  gl_Position = vec4(nodeClip.xy + ndcOffset, 0.0, 1.0);

  v_uv = localPos;
  v_nodeCenter = vec2(0.0);
  v_nodeRadius = nodeRadiusPixels;
  v_labelCenter = labelOffset;
  v_labelHalfSize = labelHalfSize;
  v_aaWidth = 1.0;
  v_shapeId = shapeId;
  v_labelAngle = labelAngle;
  v_backdropColor = a_backdropColor;
  v_backdropShadowColor = a_backdropShadowColor;
  v_backdropShadowBlur = a_backdropShadowBlur;
  v_backdropPadding = a_backdropPadding;
  v_backdropBorderColor = a_backdropBorderColor;
  v_backdropBorderWidth = borderWidth;
  v_backdropCornerRadius = cornerRadius;
  v_backdropArea = backdropArea;
}
`);return l}function cs(e){var t=e.shapes,n=e.layers,r=e.shapeGlobalIds,a=Ar(t),i=Ht(t).map(function(f){return"uniform ".concat(f.type," ").concat(f.name,";")}).join(`
`),o=function(h){return yn(h,"nodeUV","1.0")},s;if(t.length===1)s="float nodeSdfNormalized = ".concat(o(t[0]),";");else{var l=t.map(function(f,h){var g=r?r[h]:h;return"    case ".concat(g,": nodeSdfNormalized = ").concat(o(f),"; break;")}).join(`
`),u=o(t[0]);s=`float nodeSdfNormalized;
  int shapeId = int(v_shapeId);
  switch (shapeId) {
`.concat(l,`
    default: nodeSdfNormalized = `).concat(u,`;
  }`)}var c=`float effectiveRadius = max(enlargedRadius - cornerRadius, 0.01);
  float ca = u_cameraAngle * v_nodeRotation;
  float ca_c = cos(ca);
  float ca_s = sin(ca);
  vec2 rotatedScreenUV = mat2(ca_c, -ca_s, ca_s, ca_c) * screenUV;
  vec2 nodeUV = vec2(rotatedScreenUV.x, -rotatedScreenUV.y) / effectiveRadius;`,d=`#version 300 es
precision highp float;

in vec2 v_uv;
in vec2 v_nodeCenter;
in float v_nodeRadius;
in vec2 v_labelCenter;
in vec2 v_labelHalfSize;
in float v_aaWidth;
in float v_shapeId;
in float v_nodeRotation;
in float v_labelAngle;
in vec4 v_backdropColor;
in vec4 v_backdropShadowColor;
in float v_backdropShadowBlur;
in float v_backdropPadding;
in vec4 v_backdropBorderColor;
in float v_backdropBorderWidth;
in float v_backdropCornerRadius;
in float v_backdropArea;
`.concat(ls(t,n,"in"),`

uniform float u_cameraAngle;
`).concat(i,`

layout(location = 0) out vec4 fragColor;
layout(location = 1) out vec4 fragPicking;

`).concat(a,`
`).concat(Ko,`
`).concat(Zo,`
`).concat(Jo,`
`).concat(Qo,`

void main() {
  vec4 backdropColor = v_backdropColor;
  vec4 shadowColor = v_backdropShadowColor;
  float shadowBlur = v_backdropShadowBlur;
  float padding = v_backdropPadding;
  vec4 borderColor = v_backdropBorderColor;
  float borderWidth = v_backdropBorderWidth;
  float cornerRadius = v_backdropCornerRadius;

  float enlargedRadius = v_nodeRadius + padding;
  vec2 screenUV = v_uv - v_nodeCenter;
  `).concat(c,`

  // Query the correct shape SDF based on shapeId
  `).concat(s,`
  float nodeSdfPixels = nodeSdfNormalized * effectiveRadius - cornerRadius;

  // Label SDF with optional corner radius and rotation
  float labelSdfPixels;
  if (v_labelHalfSize.x > 0.0) {
    vec2 labelP = v_uv - v_labelCenter;
    labelSdfPixels = sdfRoundedRotatedBox(labelP, v_labelHalfSize, v_labelAngle, cornerRadius);
  } else {
    labelSdfPixels = 10000.0;
  }

  // Select area: 0=both, 1=node, 2=label
  float combinedSdf;
  if (v_backdropArea > 1.5) {
    combinedSdf = labelSdfPixels;
  } else if (v_backdropArea > 0.5) {
    combinedSdf = nodeSdfPixels;
  } else {
    combinedSdf = min(nodeSdfPixels, labelSdfPixels);
  }

  // Fill + border composite
  float outerEdge = smoothstep(v_aaWidth, -v_aaWidth, combinedSdf);
  vec4 background;
  if (borderWidth > 0.5) {
    float innerEdge = smoothstep(v_aaWidth, -v_aaWidth, combinedSdf + borderWidth);
    float fillAlpha = innerEdge * backdropColor.a;
    vec4 fill = vec4(backdropColor.rgb * fillAlpha, fillAlpha);
    float borderAlpha = (outerEdge - innerEdge) * borderColor.a;
    vec4 border = vec4(borderColor.rgb * borderAlpha, borderAlpha);
    background = fill + border * (1.0 - fill.a);
  } else {
    float fillAlpha = outerEdge * backdropColor.a;
    background = vec4(backdropColor.rgb * fillAlpha, fillAlpha);
  }

  // Gaussian-like shadow falloff (mimics canvas shadowBlur)
  vec4 shadow = vec4(0.0);
  float sigma = shadowBlur / 2.5;
  if (sigma > 0.001) {
    float shadowDist = max(0.0, combinedSdf);
    float shadowAlpha = exp(-(shadowDist * shadowDist) / (2.0 * sigma * sigma)) * shadowColor.a;
    shadow = vec4(shadowColor.rgb * shadowAlpha, shadowAlpha);
  }

  fragColor = background + shadow * (1.0 - background.a);
  fragPicking = vec4(0.0);
}
`);return d}function ds(e){var t=["u_matrix","u_sizeRatio","u_correctionRatio","u_cameraAngle","u_resolution","u_pixelRatio","u_labelMargin","u_zoomLabelSizeRatio","u_labelPixelSnapping","u_nodeDataTexture","u_nodeDataTextureWidth","u_nodeFrameTexture","u_nodeFrameTextureWidth","u_layerAttributeTexture","u_layerAttributeTextureWidth","u_layerAttributeTexelsPerNode"],n=G(Ht(e)),r;try{for(n.s();!(r=n.n()).done;){var a=r.value;t.includes(a.name)||t.push(a.name)}}catch(i){n.e(i)}finally{n.f()}return t}function hs(e){return{vertexShader:us(e),fragmentShader:cs(e),uniforms:ds(e.shapes)}}function fs(e,t,n,r){var a,i,o=r.label,s=o===void 0?{}:o,l=r.shapes,u=r.layers,c=r.getAttributeTexture,d=r.shapeGlobalIds;if(l.length===0)throw new Error("createBackdropProgram: at least one shape must be provided in 'shapes'");var f=(a=s.margin)!==null&&a!==void 0?a:5,h=(i=s.zoomToLabelSizeRatioFunction)!==null&&i!==void 0?i:function(){return 1},g={shapes:l,layers:u,shapeGlobalIds:d},m=hs(g),v=(function(p){function b(){var y;K(this,b);for(var _=arguments.length,x=new Array(_),T=0;T<_;T++)x[T]=arguments[T];return y=ae(this,b,[].concat(x)),R(y,"totalBackdropCount",0),R(y,"bufferCapacity",0),y}return ie(b,p),Z(b,[{key:"getDefinition",value:function(){var _=WebGL2RenderingContext,x=_.FLOAT,T=_.TRIANGLE_STRIP;return{VERTICES:4,VERTEX_SHADER_SOURCE:m.vertexShader,FRAGMENT_SHADER_SOURCE:m.fragmentShader,METHOD:T,UNIFORMS:m.uniforms,ATTRIBUTES:[{name:"a_nodeIndex",size:1,type:x},{name:"a_labelWidth",size:1,type:x},{name:"a_labelHeight",size:1,type:x},{name:"a_textHeight",size:1,type:x},{name:"a_positionMode",size:1,type:x},{name:"a_labelAngle",size:1,type:x},{name:"a_backdropColor",size:4,type:x},{name:"a_backdropShadowColor",size:4,type:x},{name:"a_backdropShadowBlur",size:1,type:x},{name:"a_backdropPadding",size:1,type:x},{name:"a_backdropBorderColor",size:4,type:x},{name:"a_backdropExtra",size:4,type:x},{name:"a_labelBoxOffset",size:2,type:x}],CONSTANT_ATTRIBUTES:[{name:"a_quadCorner",size:2,type:x}],CONSTANT_DATA:[[-1,-1],[1,-1],[-1,1],[1,1]]}}},{key:"processBackdrop",value:function(_,x){var T=this.floats,E=this.STRIDE,S=_*E;T[S++]=x.nodeIndex,T[S++]=x.labelWidth,T[S++]=x.labelHeight,T[S++]=x.textHeight,T[S++]=mt[x.position],T[S++]=x.labelAngle,T[S++]=x.backdropColor[0],T[S++]=x.backdropColor[1],T[S++]=x.backdropColor[2],T[S++]=x.backdropColor[3],T[S++]=x.backdropShadowColor[0],T[S++]=x.backdropShadowColor[1],T[S++]=x.backdropShadowColor[2],T[S++]=x.backdropShadowColor[3],T[S++]=x.backdropShadowBlur,T[S++]=x.backdropPadding,T[S++]=x.backdropBorderColor[0],T[S++]=x.backdropBorderColor[1],T[S++]=x.backdropBorderColor[2],T[S++]=x.backdropBorderColor[3],T[S++]=x.backdropBorderWidth,T[S++]=x.backdropCornerRadius,T[S++]=x.backdropLabelPadding,T[S++]=x.backdropArea,T[S++]=x.labelBoxOffset[0],T[S++]=x.labelBoxOffset[1]}},{key:"setUniforms",value:function(_,x){var T=x.gl,E=x.uniformLocations;T.uniformMatrix3fv(E.u_matrix,!1,_.matrix),T.uniform1f(E.u_sizeRatio,_.sizeRatio),T.uniform1f(E.u_correctionRatio,_.correctionRatio),T.uniform1f(E.u_cameraAngle,_.cameraAngle),T.uniform2f(E.u_resolution,_.width*_.pixelRatio,_.height*_.pixelRatio),T.uniform1f(E.u_pixelRatio,_.pixelRatio),T.uniform1f(E.u_labelMargin,b.labelMargin),T.uniform1f(E.u_zoomLabelSizeRatio,1/h(_.zoomRatio)),T.uniform1f(E.u_labelPixelSnapping,_.labelPixelSnapping),T.uniform1i(E.u_nodeDataTexture,_.nodeDataTextureUnit),T.uniform1i(E.u_nodeDataTextureWidth,_.nodeDataTextureWidth),T.uniform1i(E.u_nodeFrameTexture,_.nodeFrameTextureUnit),T.uniform1i(E.u_nodeFrameTextureWidth,_.nodeFrameTextureWidth);var S=c();E.u_layerAttributeTexture&&S&&(S.bind(et),T.uniform1i(E.u_layerAttributeTexture,et),T.uniform1i(E.u_layerAttributeTextureWidth,S.getTextureWidth()),T.uniform1i(E.u_layerAttributeTexelsPerNode,S.getTexelsPerItem()));var w=G(Ht(l)),A;try{for(w.s();!(A=w.n()).done;){var C=A.value;this.setTypedUniform(C,x)}}catch(D){w.e(D)}finally{w.f()}}},{key:"hasNothingToRender",value:function(){return this.totalBackdropCount===0}},{key:"drawWebGL",value:function(_,x){var T=x.gl;this.totalBackdropCount!==0&&(this.isInstanced?T.drawArraysInstanced(_,0,this.VERTICES,this.totalBackdropCount):T.drawArrays(_,0,this.totalBackdropCount*this.VERTICES))}},{key:"reallocate",value:function(_){this.totalBackdropCount=_,_>this.bufferCapacity&&(this.bufferCapacity=Math.max(_,Math.ceil(this.bufferCapacity*1.5)||10),de(b,"reallocate",this,3)([this.bufferCapacity]))}}])})(rt);return R(v,"labelMargin",f),new v(e,t,n)}var Ba=(function(e){function t(){var n;K(this,t);for(var r=arguments.length,a=new Array(r),i=0;i<r;i++)a[i]=arguments[i];return n=ae(this,t,[].concat(a)),R(n,"totalCharacterCount",0),R(n,"bufferCapacity",0),n}return ie(t,e),Z(t,[{key:"processLabel",value:function(r,a,i){if(i.hidden||!i.text)return 0;for(var o=i.text,s=o.length,l=0;l<s;l++){var u=o[l];this.processCharacter(a+l,i,u,l)}return s}},{key:"hasNothingToRender",value:function(){return this.totalCharacterCount===0}},{key:"drawWebGL",value:function(r,a){var i=a.gl;this.totalCharacterCount!==0&&(this.isInstanced?i.drawArraysInstanced(r,0,this.VERTICES,this.totalCharacterCount):i.drawArrays(r,0,this.totalCharacterCount*this.VERTICES))}},{key:"reallocate",value:function(r){this.totalCharacterCount=r,r>this.bufferCapacity&&(this.bufferCapacity=Math.max(r,Math.ceil(this.bufferCapacity*1.5)||1e3),de(t,"reallocate",this,3)([this.bufferCapacity]))}}])})(rt);function Ch(){var e=`#version 300 es

in float a_nodeIndex;
in vec4 a_id;
in vec4 a_color;
in float a_labelWidth;
in float a_labelHeight;
in float a_textHeight;
in float a_positionMode;
in float a_labelAngle;
in float a_padding;
in vec2 a_quadCorner;

uniform mat3 u_matrix;
uniform float u_sizeRatio;
uniform float u_correctionRatio;
uniform float u_cameraAngle;
uniform vec2 u_resolution;
uniform float u_pixelRatio;
uniform float u_labelMargin;
uniform float u_zoomLabelSizeRatio;
uniform float u_labelPixelSnapping;
uniform float u_pickingPadding;
uniform sampler2D u_nodeDataTexture;
uniform int u_nodeDataTextureWidth;
uniform sampler2D u_nodeFrameTexture;
uniform int u_nodeFrameTextureWidth;

out vec4 v_id;
out vec4 v_color;

`.concat(Be,`
`).concat(nt,`
`).concat(yt,`
`).concat(Ma,`

void main() {
  int nodeIdx = int(a_nodeIndex);

  // Node data: (x, y, size, shapeId).
  vec4 nodeData = readNodeData(u_nodeDataTexture, u_nodeDataTextureWidth, nodeIdx);
  vec2 nodePosition = nodeData.xy;
  float nodeSize = nodeData.z;

  // Shape-aware edge distance, read once from the shared frame texture.
  float edgeDist = readFrameTexel(u_nodeFrameTexture, u_nodeFrameTextureWidth, nodeIdx).r;

  // Per-node label rotation alignment: 0 = viewport, 1 = label turns with camera.
  float labelRotation = readNodeFlags(u_nodeDataTexture, u_nodeDataTextureWidth, nodeIdx).g;

  `).concat(Na,`

  float zoomScale = u_zoomLabelSizeRatio;
  float labelW = a_labelWidth * zoomScale * u_pixelRatio;
  float labelH = a_labelHeight * zoomScale * u_pixelRatio;
  float labelMargin = u_labelMargin * zoomScale * u_pixelRatio;
#ifdef PICKING_MODE
  float padding = u_pickingPadding * u_pixelRatio;
#else
  float padding = a_padding * u_pixelRatio;
#endif

  if (labelW <= 0.0) {
    gl_Position = vec4(2.0, 0.0, 0.0, 1.0);
    v_id = vec4(0.0);
    v_color = vec4(0.0);
    return;
  }

  vec2 labelHalfSize = vec2(labelW * 0.5 + padding, labelH * 0.5 + padding);
  vec2 labelOffset = vec2(0.0);

  // Graph-aligned labels add the camera angle so the rect orbits with the text.
  float labelAngle = a_labelAngle - labelRotation * u_cameraAngle;
  float la_c = cos(labelAngle);
  float la_s = sin(labelAngle);
  mat2 labelRotMat = mat2(la_c, -la_s, la_s, la_c);

  vec3 nodeClip = u_matrix * vec3(nodePosition, 1.0);
  vec2 nodeScreen = vec2(
    (nodeClip.x + 1.0) * u_resolution.x,
    (1.0 - nodeClip.y) * u_resolution.y
  ) * 0.5;
  vec2 snapDelta = (round(nodeScreen) - nodeScreen) * u_labelPixelSnapping;

  if (a_positionMode < 4.0) {
    float labelStart = nodeRadiusPixels * edgeDist + labelMargin;
    float textHalf = a_textHeight * zoomScale * u_pixelRatio * 0.5;
    // Box center uses the text half-size; the padding expands the quad below.
    labelOffset = labelRotMat * labelBoxCenter(a_positionMode, labelStart, vec2(labelW * 0.5, labelH * 0.5), textHalf);
  }

  // Rotate the rect with the label so it stays aligned with the (rotated) text.
  vec2 localPos = labelOffset + labelRotMat * (a_quadCorner * labelHalfSize);
  vec2 ndcOffset = (localPos + snapDelta) * 2.0 / u_resolution;
  ndcOffset.y = -ndcOffset.y;

  gl_Position = vec4(nodeClip.xy + ndcOffset, 0.0, 1.0);
  v_id = a_id;
  v_color = a_color;
}
`);return e}var kh=`#version 300 es
precision highp float;

in vec4 v_id;
in vec4 v_color;

out vec4 fragColor;

void main() {
  #ifdef PICKING_MODE
    fragColor = v_id;
  #else
    if (v_color.a <= 0.0) discard;
    // v_color is non-premultiplied RGBA (0-1); convert to premultiplied for blending
    fragColor = vec4(v_color.rgb * v_color.a, v_color.a);
  #endif
}
`;function Lh(e,t,n,r){var a,i,o=r.label,s=o===void 0?{}:o,l=(a=s.margin)!==null&&a!==void 0?a:bt,u=(i=s.zoomToLabelSizeRatioFunction)!==null&&i!==void 0?i:function(){return 1},c=Ch(),d=(function(f){function h(){var g;K(this,h);for(var m=arguments.length,v=new Array(m),p=0;p<m;p++)v[p]=arguments[p];return g=ae(this,h,[].concat(v)),R(g,"totalCount",0),R(g,"bufferCapacity",0),g}return ie(h,f),Z(h,[{key:"getDefinition",value:function(){var m=WebGL2RenderingContext,v=m.FLOAT,p=m.UNSIGNED_BYTE,b=m.TRIANGLE_STRIP;return{VERTICES:4,VERTEX_SHADER_SOURCE:c,FRAGMENT_SHADER_SOURCE:kh,METHOD:b,UNIFORMS:["u_matrix","u_sizeRatio","u_correctionRatio","u_cameraAngle","u_resolution","u_pixelRatio","u_labelMargin","u_zoomLabelSizeRatio","u_labelPixelSnapping","u_pickingPadding","u_nodeDataTexture","u_nodeDataTextureWidth","u_nodeFrameTexture","u_nodeFrameTextureWidth"],ATTRIBUTES:[{name:"a_nodeIndex",size:1,type:v},{name:"a_id",size:4,type:p,normalized:!0},{name:"a_color",size:4,type:p,normalized:!0},{name:"a_labelWidth",size:1,type:v},{name:"a_labelHeight",size:1,type:v},{name:"a_textHeight",size:1,type:v},{name:"a_positionMode",size:1,type:v},{name:"a_labelAngle",size:1,type:v},{name:"a_padding",size:1,type:v}],CONSTANT_ATTRIBUTES:[{name:"a_quadCorner",size:2,type:v}],CONSTANT_DATA:[[-1,-1],[1,-1],[-1,1],[1,1]]}}},{key:"processLabelBackground",value:function(m,v){var p=this.floats,b=this.ints,y=m*this.STRIDE;p[y++]=v.nodeIndex,b[y++]=v.id,p[y++]=v.color,p[y++]=v.labelWidth,p[y++]=v.labelHeight,p[y++]=v.textHeight,p[y++]=v.positionMode,p[y++]=v.labelAngle,p[y++]=v.padding}},{key:"setUniforms",value:function(m,v){var p=v.gl,b=v.uniformLocations;p.uniformMatrix3fv(b.u_matrix,!1,m.matrix),p.uniform1f(b.u_sizeRatio,m.sizeRatio),p.uniform1f(b.u_correctionRatio,m.correctionRatio),p.uniform1f(b.u_cameraAngle,m.cameraAngle),p.uniform2f(b.u_resolution,m.width*m.pixelRatio,m.height*m.pixelRatio),p.uniform1f(b.u_pixelRatio,m.pixelRatio),p.uniform1f(b.u_labelMargin,h.labelMargin),p.uniform1f(b.u_zoomLabelSizeRatio,1/u(m.zoomRatio)),p.uniform1f(b.u_labelPixelSnapping,m.labelPixelSnapping),p.uniform1f(b.u_pickingPadding,m.labelPickingPadding),p.uniform1i(b.u_nodeDataTexture,m.nodeDataTextureUnit),p.uniform1i(b.u_nodeDataTextureWidth,m.nodeDataTextureWidth),p.uniform1i(b.u_nodeFrameTexture,m.nodeFrameTextureUnit),p.uniform1i(b.u_nodeFrameTextureWidth,m.nodeFrameTextureWidth)}},{key:"hasNothingToRender",value:function(){return this.totalCount===0}},{key:"drawWebGL",value:function(m,v){var p=v.gl;this.totalCount!==0&&p.drawArraysInstanced(p.TRIANGLE_STRIP,0,this.VERTICES,this.totalCount)}},{key:"reallocate",value:function(m){this.totalCount=m,m>this.bufferCapacity&&(this.bufferCapacity=Math.max(m,Math.ceil(this.bufferCapacity*1.5)||10),de(h,"reallocate",this,3)([this.bufferCapacity]))}}])})(rt);return R(d,"labelMargin",l),new d(e,t,n)}var Ge={fontSize:64,buffer:8,radius:24,cutoff:.25,maxTextureSize:2048,debounceTimeout:100},pn=2,mn=1e20;function Fh(e,t,n,r,a){var i=e+a*4,o=document.createElement("canvas");o.width=i,o.height=i;var s=o.getContext("2d",{willReadFrequently:!0});return s.font="".concat(r," ").concat(n," ").concat(e,"px ").concat(t),s.textBaseline="alphabetic",s.textAlign="left",s.fillStyle="black",{ctx:s,canvasSize:i,gridOuter:new Float64Array(i*i),gridInner:new Float64Array(i*i),f:new Float64Array(i),z:new Float64Array(i+1),v:new Uint16Array(i)}}function Ph(e,t,n,r,a){var i=e.ctx,o=e.canvasSize,s=i.measureText(t),l=s.width,u=s.actualBoundingBoxAscent,c=s.actualBoundingBoxDescent,d=s.actualBoundingBoxLeft,f=s.actualBoundingBoxRight,h=Math.ceil(u),g=Math.ceil(d),m=Math.max(0,Math.min(o-n,Math.ceil(d)+Math.ceil(f))),v=Math.min(o-n,h+Math.ceil(c)),p=m+2*n,b=v+2*n,y=Math.max(p*b,0),_=new Uint8ClampedArray(y),x={data:_,width:p,height:b,glyphWidth:m,glyphHeight:v,glyphTop:h,glyphLeft:g,glyphAdvance:l};if(m===0||v===0)return x;var T=e.gridInner,E=e.gridOuter;i.clearRect(n,n,m,v),i.fillText(t,n+g,n+h);var S=i.getImageData(n,n,m,v);E.fill(mn,0,y),T.fill(0,0,y);for(var w=0;w<v;w++)for(var A=0;A<m;A++){var C=S.data[4*(w*m+A)+3]/255;if(C!==0){var D=(w+n)*p+A+n;if(C===1)E[D]=0,T[D]=mn;else{var F=.5-C;E[D]=F>0?F*F:0,T[D]=F<0?F*F:0}}}zo(E,0,0,p,b,p,e.f,e.v,e.z),zo(T,n,n,m,v,p,e.f,e.v,e.z);for(var k=0;k<y;k++){var L=Math.sqrt(E[k])-Math.sqrt(T[k]);_[k]=Math.round(255-255*(L/r+a))}return x}function zo(e,t,n,r,a,i,o,s,l){for(var u=t;u<t+r;u++)Mo(e,n*i+u,i,a,o,s,l);for(var c=n;c<n+a;c++)Mo(e,c*i+t,1,r,o,s,l)}function Mo(e,t,n,r,a,i,o){i[0]=0,o[0]=-mn,o[1]=mn,a[0]=e[t];for(var s=1,l=0,u=0;s<r;s++){a[s]=e[t+s*n];var c=s*s;do{var d=i[l];u=(a[s]-a[d]+c-d*d)/(s-d)/2}while(u<=o[l]&&--l>-1);l++,i[l]=s,o[l]=u,o[l+1]=mn}for(var f=0,h=0;f<r;f++){for(;o[h+1]<f;)h++;var g=i[h],m=f-g;e[t+f*n]=a[g]+m*m}}var vt=(function(e){function t(){var n,r=arguments.length>0&&arguments[0]!==void 0?arguments[0]:{};return K(this,t),n=ae(this,t),R(n,"fonts",new Map),R(n,"textures",[]),R(n,"cursor",{x:0,y:0,rowHeight:0,atlasIndex:0}),R(n,"pendingGlyphs",[]),R(n,"debounceTimer",null),n.options=B(B({},Ge),r),n.canvas=document.createElement("canvas"),n.canvas.width=n.options.maxTextureSize,n.canvas.height=n.options.maxTextureSize,n.ctx=n.canvas.getContext("2d",{willReadFrequently:!0}),n.measureCanvas=document.createElement("canvas"),n.measureCtx=n.measureCanvas.getContext("2d"),n.textures.push(n.ctx.getImageData(0,0,1,1)),n}return ie(t,e),Z(t,[{key:"getFontKey",value:function(r){return"".concat(r.family,"-").concat(r.weight,"-").concat(r.style)}},{key:"registerFont",value:function(r){var a=this.getFontKey(r);if(this.fonts.has(a))return a;var i=Fh(this.options.fontSize,r.family,r.weight,r.style,this.options.buffer);return this.fonts.set(a,{descriptor:r,generator:i,glyphs:new Map}),a}},{key:"ensureGlyphs",value:function(r,a){var i=this.fonts.get(a);if(!i)throw new Error('Font "'.concat(a,'" is not registered. Call registerFont() first.'));var o=!1,s=G(r),l;try{for(s.s();!(l=s.n()).done;){var u=l.value,c=u.codePointAt(0);c!==void 0&&(i.glyphs.has(c)||(this.pendingGlyphs.push({fontKey:a,charCode:c}),o=!0))}}catch(d){s.e(d)}finally{s.f()}o&&this.scheduleTextureGeneration()}},{key:"measureText",value:function(r,a){var i=this.fonts.get(a);if(!i)throw new Error('Font "'.concat(a,'" is not registered.'));var o=i.descriptor,s=o.family,l=o.weight,u=o.style;return this.measureCtx.font="".concat(u," ").concat(l," ").concat(this.options.fontSize,"px ").concat(s),this.measureCtx.measureText(r).width}},{key:"getGlyph",value:function(r,a){var i=this.fonts.get(a);if(i)return i.glyphs.get(r)}},{key:"getTextures",value:function(){return this.textures}},{key:"getFontCount",value:function(){return this.fonts.size}},{key:"getGlyphCount",value:function(){var r=0,a=G(this.fonts.values()),i;try{for(a.s();!(i=a.n()).done;){var o=i.value;r+=o.glyphs.size}}catch(s){a.e(s)}finally{a.f()}return r}},{key:"hasPendingGlyphs",value:function(){return this.pendingGlyphs.length>0}},{key:"flush",value:function(){this.debounceTimer&&(clearTimeout(this.debounceTimer),this.debounceTimer=null),this.generateTextures()}},{key:"destroy",value:function(){this.debounceTimer&&clearTimeout(this.debounceTimer),this.fonts.clear(),this.textures=[],this.pendingGlyphs=[],this.removeAllListeners()}},{key:"scheduleTextureGeneration",value:function(){var r=this;this.debounceTimer===null&&(this.options.debounceTimeout===null?this.generateTextures():this.debounceTimer=setTimeout(function(){r.debounceTimer=null,r.generateTextures()},this.options.debounceTimeout))}},{key:"generateTextures",value:function(){if(this.pendingGlyphs.length!==0){var r=this.options,a=r.maxTextureSize,i=r.buffer,o=r.radius,s=r.cutoff,l=G(this.pendingGlyphs),u;try{for(l.s();!(u=l.n()).done;){var c=u.value,d=c.fontKey,f=c.charCode,h=this.fonts.get(d);if(!(!h||h.glyphs.has(f))){var g=String.fromCodePoint(f),m=Ph(h.generator,g,i,o,s),v=m.width,p=m.height;this.cursor.x+v+pn>a&&(this.cursor.x=0,this.cursor.y+=this.cursor.rowHeight+pn,this.cursor.rowHeight=0),this.cursor.y+p+pn>a&&(this.finalizeCurrentTexture(),this.cursor={x:0,y:0,rowHeight:0,atlasIndex:this.cursor.atlasIndex+1},this.ctx.clearRect(0,0,a,a));for(var b=m.data,y=new Uint8ClampedArray(v*p*4),_=0;_<b.length;_++){var x=_*4;y[x]=255,y[x+1]=255,y[x+2]=255,y[x+3]=b[_]}var T=new ImageData(y,v,p);this.ctx.putImageData(T,this.cursor.x,this.cursor.y);var E={charCode:f,width:m.glyphWidth,height:m.glyphHeight,bearingX:-m.glyphLeft-i,bearingY:m.glyphTop+i,advance:m.glyphAdvance,atlasX:this.cursor.x,atlasY:this.cursor.y,atlasWidth:v,atlasHeight:p,atlasIndex:this.cursor.atlasIndex};h.glyphs.set(f,E),this.cursor.x+=v+pn,this.cursor.rowHeight=Math.max(this.cursor.rowHeight,p)}}}catch(S){l.e(S)}finally{l.f()}this.finalizeCurrentTexture(),this.pendingGlyphs=[],this.emit(t.ATLAS_UPDATED_EVENT,{textures:this.textures,glyphCount:this.getGlyphCount()})}}},{key:"finalizeCurrentTexture",value:function(){var r=this.options.maxTextureSize,a=this.cursor.y>0||this.cursor.rowHeight>0,i=Math.min(r,Math.max(this.cursor.x,a?r:1)),o=Math.min(r,this.cursor.y+this.cursor.rowHeight+pn),s=this.ctx.getImageData(0,0,i,o);this.cursor.atlasIndex>=this.textures.length?this.textures.push(s):this.textures[this.cursor.atlasIndex]=s}}])})(Oo.EventEmitter);R(vt,"ATLAS_UPDATED_EVENT","atlasUpdated");var Ih=Ge.fontSize;function gs(){var e=`  // -------------------------------------------------------------------------
  // Step 3: Calculate position offset from the shared edge distance
  // -------------------------------------------------------------------------
  vec2 positionOffset = vec2(0.0);

  if (a_positionMode < 4.0) {
    vec2 screenDir = getLabelDirection(a_positionMode);
    float boundaryDistPixels = nodeRadiusPixels * edgeDist;
    positionOffset = screenDir * (boundaryDistPixels + margin);
  }`,t=`#version 300 es

// ============================================================================
// Attributes
// ============================================================================

// Per-character (instanced)
in float a_nodeIndex;        // Index into node data texture
in vec2 a_charOffset;        // Character offset from label origin (pixels)
in vec2 a_charSize;          // Character dimensions (pixels)
in vec4 a_texCoords;         // Atlas coords: (x, y, width, height) in pixels
in vec4 a_color;             // Text color (RGBA)
in float a_margin;           // Gap between node edge and label (pixels)
in float a_positionMode;     // Position: 0=right, 1=left, 2=above, 3=below, 4=over
in float a_labelWidth;       // Total label width (pixels)
in float a_labelHeight;      // Label height (pixels)
in float a_verticalCenter;   // Vertical center offset from baseline (pixels)
in float a_textHeight;       // Actual text height: maxAscent + maxDescent (pixels)
in float a_labelAngle;       // Label rotation angle (radians)

// Per-vertex (constant quad corners)
in vec2 a_quadCorner;        // Quad corner: [-1,-1], [1,-1], [-1,1], [1,1]

// ============================================================================
// Uniforms
// ============================================================================

uniform mat3 u_matrix;
uniform float u_sizeRatio;
uniform float u_correctionRatio;
uniform float u_cameraAngle;
uniform vec2 u_resolution;
uniform vec2 u_atlasSize;
uniform sampler2D u_nodeDataTexture;
uniform int u_nodeDataTextureWidth;
uniform sampler2D u_nodeFrameTexture;
uniform int u_nodeFrameTextureWidth;
uniform float u_zoomLabelSizeRatio;
uniform float u_labelPixelSnapping;
uniform float u_pixelRatio;

// ============================================================================
// Varyings
// ============================================================================

out vec2 v_texCoord;
out vec4 v_color;
out float v_fontScale;

// ============================================================================
// Constants
// ============================================================================

const float bias = 255.0 / 254.0;
const float ATLAS_FONT_SIZE = `.concat(re(Ih),`;

// ============================================================================
// Helper Functions
// ============================================================================

`).concat(Be,`
`).concat(nt,`
`).concat(yt,`
`).concat(za,`

// ============================================================================
// Main
// ============================================================================

void main() {
  // -------------------------------------------------------------------------
  // Step 0: Fetch node data + shared edge distance from textures
  // -------------------------------------------------------------------------
  // Node-data texture format: vec4(x, y, size, shapeId)
  // 2D texture layout: texCoord = (index % width, index / width)
  int nodeIdx = int(a_nodeIndex);
  vec4 nodeData = readNodeData(u_nodeDataTexture, u_nodeDataTextureWidth, nodeIdx);
  vec2 a_anchorPosition = nodeData.xy;
  float a_nodeSize = nodeData.z;

  // Normalized edge distance from the shared frame texture (the frame-pass ran
  // the SDF search once; the label just reads the result).
  float edgeDist = readFrameTexel(u_nodeFrameTexture, u_nodeFrameTextureWidth, nodeIdx).r;

  // Per-node label rotation alignment: 0 = viewport, 1 = label turns with camera.
  float labelRotation = readNodeFlags(u_nodeDataTexture, u_nodeDataTextureWidth, nodeIdx).g;

  // Apply zoom-dependent label size scaling
  // Positional values are in CSS pixels; multiply by u_pixelRatio to convert to
  // physical pixels, which is what the NDC conversion (/ u_resolution) expects.
  float zoomScale = u_zoomLabelSizeRatio;
  float margin = a_margin * zoomScale * u_pixelRatio;
  vec2 charOffset = a_charOffset * zoomScale * u_pixelRatio;
  vec2 charSize = a_charSize * zoomScale * u_pixelRatio;
  float labelWidth = a_labelWidth * zoomScale * u_pixelRatio;
  float labelHeight = a_labelHeight * zoomScale;

  // Font scale: ratio of CSS label size to the base atlas font size.
  // Divided by u_pixelRatio because the atlas is generated at ATLAS_FONT_SIZE * pixelRatio,
  // which cancels out the u_pixelRatio in the fragment shader's gamma formula and keeps
  // the anti-aliasing band width consistent across pixel densities.
  v_fontScale = a_labelHeight * zoomScale / (ATLAS_FONT_SIZE * u_pixelRatio);

  // -------------------------------------------------------------------------
  // Step 1: Transform node position to clip space
  // -------------------------------------------------------------------------
  vec3 anchorClip = u_matrix * vec3(a_anchorPosition, 1.0);

  // -------------------------------------------------------------------------
  // Step 2: Convert node size to screen pixels
  // -------------------------------------------------------------------------
  float matrixScaleX = length(vec2(u_matrix[0][0], u_matrix[1][0]));
  float nodeRadiusGraphSpace = a_nodeSize * u_correctionRatio / u_sizeRatio * 2.0;
  float nodeRadiusNDC = nodeRadiusGraphSpace * matrixScaleX;
  float nodeRadiusPixels = nodeRadiusNDC * u_resolution.x / 2.0;

`).concat(e,`

  // -------------------------------------------------------------------------
  // Step 4: Calculate final vertex position
  // -------------------------------------------------------------------------
  vec2 cornerOffset = (a_quadCorner + 1.0) * 0.5;
  vec2 charPixelPos = positionOffset + charOffset + cornerOffset * charSize;

  // Apply text alignment based on position mode
  float verticalCenter = a_verticalCenter * zoomScale * u_pixelRatio;
  float textHeight = a_textHeight * zoomScale * u_pixelRatio;
  float baselineToDescent = textHeight / 2.0 - verticalCenter;
  float baselineToAscent = textHeight / 2.0 + verticalCenter;

  if (a_positionMode < 0.5) {
    // Right: vertically center
    charPixelPos.y += verticalCenter;
  } else if (a_positionMode < 1.5) {
    // Left: right-align and vertically center
    charPixelPos.x -= labelWidth;
    charPixelPos.y += verticalCenter;
  } else if (a_positionMode < 2.5) {
    // Above: center horizontally, bottom of text at anchor
    charPixelPos.x -= labelWidth * 0.5;
    charPixelPos.y -= baselineToDescent;
  } else if (a_positionMode < 3.5) {
    // Below: center horizontally, top of text at anchor
    charPixelPos.x -= labelWidth * 0.5;
    charPixelPos.y += baselineToAscent;
  } else {
    // Over: center both
    charPixelPos.x -= labelWidth * 0.5;
    charPixelPos.y += verticalCenter;
  }

  // Apply label angle rotation. Graph-aligned labels add the camera angle so the
  // whole label orbits the node in lockstep with the frame-pass edge distance.
  float labelAngle = a_labelAngle - labelRotation * u_cameraAngle;
  float la_c = cos(labelAngle);
  float la_s = sin(labelAngle);
  mat2 labelRotMat = mat2(la_c, -la_s, la_s, la_c);
  charPixelPos = labelRotMat * charPixelPos;

  // Snap node center to pixel grid so label/backdrop/attachment move as a unit
  vec2 nodeScreen = vec2(
    (anchorClip.x + 1.0) * u_resolution.x,
    (1.0 - anchorClip.y) * u_resolution.y
  ) * 0.5;
  charPixelPos += (round(nodeScreen) - nodeScreen) * u_labelPixelSnapping;

  // Convert to NDC (flip Y: screen Y-down -> clip Y-up)
  vec2 ndcOffset = vec2(charPixelPos.x, -charPixelPos.y) * 2.0 / u_resolution;
  gl_Position = vec4(anchorClip.xy + ndcOffset, 0.0, 1.0);

  // -------------------------------------------------------------------------
  // Step 5: Texture coordinates
  // -------------------------------------------------------------------------
  v_texCoord = (a_texCoords.xy + cornerOffset * a_texCoords.zw) / u_atlasSize;

  // -------------------------------------------------------------------------
  // Step 6: Pass color
  // -------------------------------------------------------------------------
  v_color = a_color;
  v_color.a *= bias;
}
`);return t}function ps(){var e=`#version 300 es
precision highp float;

in vec2 v_texCoord;
in vec4 v_color;
in float v_fontScale;

uniform sampler2D u_atlas;
uniform float u_gamma;
uniform float u_sdfBuffer;
uniform float u_pixelRatio;

// Fragment output (single target - picking handled via separate pass)
out vec4 fragColor;

void main() {
  #ifdef PICKING_MODE
    // Labels are not pickable - discard all fragments in picking mode
    fragColor = vec4(0.0);
    discard;
  #else
    // Sample SDF value from atlas (high = inside glyph, low = outside)
    float sdfValue = texture(u_atlas, v_texCoord).a;

    // Edge threshold: 1.0 - cutoff = 0.75 for default cutoff=0.25
    // This is where the glyph edge is located in the SDF
    float edgeThreshold = 1.0 - u_sdfBuffer;

    // Gamma controls the anti-aliasing band width.
    // Scale inversely with font scale so small labels get a wider AA band
    // (smoother) and large labels get a tighter band (sharper).
    float gamma = u_gamma / (u_pixelRatio * v_fontScale);

    // Pure gamma-based anti-aliasing using smoothstep
    // The AA band extends from (threshold - gamma) to (threshold + gamma)
    float alpha = smoothstep(edgeThreshold - gamma, edgeThreshold + gamma, sdfValue);

    // Premultiplied alpha output for correct blending
    float finalAlpha = v_color.a * alpha;
    fragColor = vec4(v_color.rgb * finalAlpha, finalAlpha);
  #endif
}
`;return e}function vs(){return["u_matrix","u_sizeRatio","u_correctionRatio","u_cameraAngle","u_resolution","u_atlasSize","u_atlas","u_gamma","u_sdfBuffer","u_pixelRatio","u_nodeDataTexture","u_nodeDataTextureWidth","u_nodeFrameTexture","u_nodeFrameTextureWidth","u_zoomLabelSizeRatio","u_labelPixelSnapping"]}function ms(){return{vertexShader:gs(),fragmentShader:ps(),uniforms:vs()}}function bs(e,t,n,r){var a,i,o=r.label,s=o===void 0?{}:o,l=(a=s.margin)!==null&&a!==void 0?a:bt,u=(i=s.zoomToLabelSizeRatioFunction)!==null&&i!==void 0?i:function(){return 1},c=ms(),d=(function(f){function h(g,m,v){var p,b,y,_;if(K(this,h),_=ae(this,h,[g,m,v]),R(_,"atlasTexture",null),R(_,"atlasNeedsUpdate",!1),R(_,"labelGlyphCache",new Map),_.atlasFontSize=Ge.fontSize*gn(),_.atlasManager=new vt({fontSize:_.atlasFontSize}),_.gamma=.025,_.sdfBuffer=Ge.cutoff,_.atlasTexture=g.createTexture(),!_.atlasTexture)throw new Error("NodeLabelProgram: failed to create atlas texture");g.bindTexture(g.TEXTURE_2D,_.atlasTexture),g.texParameteri(g.TEXTURE_2D,g.TEXTURE_WRAP_S,g.CLAMP_TO_EDGE),g.texParameteri(g.TEXTURE_2D,g.TEXTURE_WRAP_T,g.CLAMP_TO_EDGE),g.texParameteri(g.TEXTURE_2D,g.TEXTURE_MIN_FILTER,g.LINEAR),g.texParameteri(g.TEXTURE_2D,g.TEXTURE_MAG_FILTER,g.LINEAR),g.bindTexture(g.TEXTURE_2D,null),_.atlasManager.on(vt.ATLAS_UPDATED_EVENT,function(){_.atlasNeedsUpdate=!0});var x={family:((p=s.font)===null||p===void 0?void 0:p.family)||"sans-serif",weight:((b=s.font)===null||b===void 0?void 0:b.weight)||"normal",style:((y=s.font)===null||y===void 0?void 0:y.style)||"normal"};return _.defaultFontKey=_.atlasManager.registerFont(x),_}return ie(h,f),Z(h,[{key:"getDefinition",value:function(){var m=WebGL2RenderingContext,v=m.FLOAT,p=m.UNSIGNED_BYTE,b=m.TRIANGLE_STRIP;return{VERTICES:4,VERTEX_SHADER_SOURCE:c.vertexShader,FRAGMENT_SHADER_SOURCE:c.fragmentShader,METHOD:b,UNIFORMS:c.uniforms,ATTRIBUTES:[{name:"a_nodeIndex",size:1,type:v},{name:"a_charOffset",size:2,type:v},{name:"a_charSize",size:2,type:v},{name:"a_texCoords",size:4,type:v},{name:"a_color",size:4,type:p,normalized:!0},{name:"a_margin",size:1,type:v},{name:"a_positionMode",size:1,type:v},{name:"a_labelWidth",size:1,type:v},{name:"a_labelHeight",size:1,type:v},{name:"a_verticalCenter",size:1,type:v},{name:"a_textHeight",size:1,type:v},{name:"a_labelAngle",size:1,type:v}],CONSTANT_ATTRIBUTES:[{name:"a_quadCorner",size:2,type:v}],CONSTANT_DATA:[[-1,-1],[1,-1],[-1,1],[1,1]]}}},{key:"prepareLabelGlyphs",value:function(m,v){if(v.hidden||!v.text){this.labelGlyphCache.delete(m);return}var p=v.text,b=v.fontKey||this.defaultFontKey;this.atlasManager.ensureGlyphs(p,b);var y=[],_=[],x=0,T=0,E=0,S=G(p),w;try{for(S.s();!(w=S.n()).done;){var A=w.value,C=A.codePointAt(0);if(C===void 0){y.push(void 0),_.push(x);continue}var D=this.atlasManager.getGlyph(C,b);y.push(D),_.push(x),D&&(x+=D.advance,T=Math.max(T,D.bearingY),E=Math.max(E,D.atlasHeight-D.bearingY))}}catch(F){S.e(F)}finally{S.f()}this.labelGlyphCache.set(m,{glyphs:y,xOffsets:_,totalWidth:x,totalHeight:T+E,verticalCenterOffset:(T-E)/2})}},{key:"processCharacter",value:function(m,v,p,b){var y=this.floats,_=this.STRIDE,x=m*_,T=this.labelGlyphCache.get(v.parentKey);if(!T||!T.glyphs[b]){for(var E=0;E<_;E++)y[x+E]=0;return}var S=T.glyphs[b],w=T.xOffsets[b],A=v.size/this.atlasFontSize,C=xe(v.color),D=x;y[D++]=v.nodeIndex,y[D++]=(w+S.bearingX)*A,y[D++]=-S.bearingY*A,y[D++]=S.atlasWidth*A,y[D++]=S.atlasHeight*A,y[D++]=S.atlasX,y[D++]=S.atlasY,y[D++]=S.atlasWidth,y[D++]=S.atlasHeight,y[D++]=C,y[D++]=v.margin,y[D++]=mt[v.position],y[D++]=T.totalWidth*A,y[D++]=v.size,y[D++]=T.verticalCenterOffset*A,y[D++]=T.totalHeight*A,y[D++]=v.labelAngle}},{key:"processLabel",value:function(m,v,p){return this.prepareLabelGlyphs(m,p),de(h,"processLabel",this,3)([m,v,p])}},{key:"updateAtlasTexture",value:function(){if(this.atlasNeedsUpdate){var m=this.normalProgram.gl,v=this.atlasManager.getTextures();if(v.length!==0){var p=v[0];m.bindTexture(m.TEXTURE_2D,this.atlasTexture),m.texImage2D(m.TEXTURE_2D,0,m.RGBA,p.width,p.height,0,m.RGBA,m.UNSIGNED_BYTE,p.data),m.bindTexture(m.TEXTURE_2D,null),this.atlasNeedsUpdate=!1}}}},{key:"setUniforms",value:function(m,v){var p=v.gl,b=v.uniformLocations;p.uniformMatrix3fv(b.u_matrix,!1,m.matrix),p.uniform1f(b.u_sizeRatio,m.sizeRatio),p.uniform1f(b.u_correctionRatio,m.correctionRatio),p.uniform1f(b.u_cameraAngle,m.cameraAngle),p.uniform2f(b.u_resolution,m.width*m.pixelRatio,m.height*m.pixelRatio);var y=this.atlasManager.getTextures();y.length>0?p.uniform2f(b.u_atlasSize,y[0].width,y[0].height):p.uniform2f(b.u_atlasSize,1,1),p.activeTexture(p.TEXTURE0),p.bindTexture(p.TEXTURE_2D,this.atlasTexture),p.uniform1i(b.u_atlas,0),b.u_nodeDataTexture!==void 0&&p.uniform1i(b.u_nodeDataTexture,m.nodeDataTextureUnit),b.u_nodeDataTextureWidth!==void 0&&p.uniform1i(b.u_nodeDataTextureWidth,m.nodeDataTextureWidth),p.uniform1i(b.u_nodeFrameTexture,m.nodeFrameTextureUnit),p.uniform1i(b.u_nodeFrameTextureWidth,m.nodeFrameTextureWidth),p.uniform1f(b.u_gamma,this.gamma),p.uniform1f(b.u_sdfBuffer,this.sdfBuffer),p.uniform1f(b.u_pixelRatio,m.pixelRatio),p.uniform1f(b.u_zoomLabelSizeRatio,1/h.zoomToLabelSizeRatioFunction(m.zoomRatio)),p.uniform1f(b.u_labelPixelSnapping,m.labelPixelSnapping)}},{key:"renderProgram",value:function(m,v){this.updateAtlasTexture(),this.atlasManager.hasPendingGlyphs()&&(this.atlasManager.flush(),this.updateAtlasTexture()),de(h,"renderProgram",this,3)([m,v])}},{key:"registerFont",value:function(m){var v=arguments.length>1&&arguments[1]!==void 0?arguments[1]:"normal",p=arguments.length>2&&arguments[2]!==void 0?arguments[2]:"normal";return this.atlasManager.registerFont({family:m,weight:v,style:p})}},{key:"getAtlasManager",value:function(){return this.atlasManager}},{key:"measureLabel",value:function(m,v,p){var b=p||this.defaultFontKey;this.atlasManager.ensureGlyphs(m,b),this.atlasManager.hasPendingGlyphs()&&this.atlasManager.flush();var y=0,_=0,x=0,T=G(m),E;try{for(T.s();!(E=T.n()).done;){var S=E.value,w=S.codePointAt(0);if(w!==void 0){var A=this.atlasManager.getGlyph(w,b);A&&(y+=A.advance,_=Math.max(_,A.bearingY),x=Math.max(x,A.atlasHeight-A.bearingY))}}}catch(D){T.e(D)}finally{T.f()}var C=v/this.atlasFontSize;return{width:y*C,height:v,textHeight:(_+x)*C}}},{key:"ensureGlyphsReady",value:function(m,v){var p=v||this.defaultFontKey,b=G(m),y;try{for(b.s();!(y=b.n()).done;){var _=y.value;this.atlasManager.ensureGlyphs(_,p)}}catch(x){b.e(x)}finally{b.f()}this.atlasManager.flush()}},{key:"kill",value:function(){var m=this.normalProgram.gl;this.atlasTexture&&(m.deleteTexture(this.atlasTexture),this.atlasTexture=null),this.atlasManager.destroy(),this.labelGlyphCache.clear(),de(h,"kill",this,3)([])}}])})(Ba);return R(d,"labelMargin",l),R(d,"zoomToLabelSizeRatioFunction",u),new d(e,t,n)}var Nh=`#version 300 es
precision highp float;

in float v_edgeDist;

// R32F target: only the .r channel is stored.
out vec4 fragColor;

void main() {
  fragColor = vec4(v_edgeDist, 0.0, 0.0, 0.0);
}
`;function zh(e,t,n){var r=Ar(e),a=Ht(e).map(function(g){return"uniform ".concat(g.type," ").concat(g.name,";")}).join(`
`),i=ts(e,n),o=i.code,s=i.multiShape,l=jt(e,t),u=Object.keys(l.specs).map(function(g){return"float v_".concat(g,";")}).join(`
`),c=Ot(l,{varPrefix:"nodeAttr",baseTexelExpr:"nodeIdx * u_layerAttributeTexelsPerNode",textureWidthUniform:"u_layerAttributeTextureWidth",textureSamplerUniform:"u_layerAttributeTexture"}),d=c.fetchCode,f=c.varyingAssignments,h=`#version 300 es
precision highp float;

uniform float u_cameraAngle;
uniform sampler2D u_nodeDataTexture;
uniform int u_nodeDataTextureWidth;
uniform float u_frameTextureWidth;
uniform float u_frameTextureHeight;
uniform sampler2D u_layerAttributeTexture;
uniform int u_layerAttributeTextureWidth;
uniform int u_layerAttributeTexelsPerNode;
`.concat(a,`

in float a_nodeIndex;     // node-data texture index (also the target frame texel)
in float a_positionMode;  // 0=right 1=left 2=above 3=below 4=over
in float a_labelAngle;    // intrinsic label angle (radians)

out float v_edgeDist;

// Shape attributes (plain globals: this pass is vertex-only)
`).concat(u,`

`).concat(Be,`
`).concat(nt,`
`).concat(r,`
`).concat(o,`
`).concat(za,`

void main() {
  int nodeIdx = int(a_nodeIndex);
`).concat(d,`
`).concat(f,`
  `).concat(s?`// Multi-shape: the shape id lives in the node-data texture's .w channel:
  g_shapeId = int(readNodeData(u_nodeDataTexture, u_nodeDataTextureWidth, nodeIdx).w);`:"// Single-shape mode, shape id not needed",`

  // Per-node rotation alignment (0 = viewport, 1 = graph).
  vec4 nodeFlags = readNodeFlags(u_nodeDataTexture, u_nodeDataTextureWidth, nodeIdx);
  float nodeRotation = nodeFlags.r;
  float labelRotation = nodeFlags.g;

  // Effective angle: intrinsic (style-given) plus the camera angle when the label
  // is graph-aligned. The companions place the box at this same angle.
  float effectiveAngle = a_labelAngle - labelRotation * u_cameraAngle;

  // The "over" mode (4) sits on the node center, so it has no edge distance.
  float edgeDist = 0.0;
  if (a_positionMode < 4.0) {
    vec2 screenDir = getLabelDirection(a_positionMode);
    // Rotate by the effective label angle. Inlined rather than via rotate2D()
    // because the shape SDFs (getShapeGLSLForShapes) already define that helper
    // for multi-shape programs, and redefining it would be a GLSL error.
    float ea_c = cos(effectiveAngle);
    float ea_s = sin(effectiveAngle);
    vec2 rotatedScreenDir = mat2(ea_c, -ea_s, ea_s, ea_c) * screenDir;
    // Screen (Y-down) -> SDF (Y-up).
    vec2 sdfDir = vec2(rotatedScreenDir.x, -rotatedScreenDir.y);
    // Counter-rotate into the shape's local frame for graph-aligned nodes, so the
    // boundary is queried against the shape's actual on-screen orientation.
    float nodeCa = -u_cameraAngle * nodeRotation;
    float nc = cos(nodeCa), ns = sin(nodeCa);
    sdfDir = mat2(nc, -ns, ns, nc) * sdfDir;
    edgeDist = findEdgeDistance(sdfDir, 1.0);
  }
  v_edgeDist = edgeDist;

  // Scatter to this node's texel center in the frame texture.
  float x = mod(a_nodeIndex, u_frameTextureWidth);
  float y = floor(a_nodeIndex / u_frameTextureWidth);
  vec2 ndc = (vec2(x, y) + 0.5) / vec2(u_frameTextureWidth, u_frameTextureHeight) * 2.0 - 1.0;
  gl_Position = vec4(ndc, 0.0, 1.0);
  gl_PointSize = 1.0;
}
`);return h}var Wa=(function(){function e(t,n){K(this,e),R(this,"uniformLocations",{});var r=n.shapes,a=n.layers,i=n.shapeGlobalIds;if(r.length===0)throw new Error("NodeLabelFramePass: at least one shape must be provided");this.gl=t,this.shapeUniforms=Ht(r),this.hasAttributeData=Object.keys(jt(r,a).offsets).length>0,this.vertexShader=Er(t,zh(r,a,i)),this.fragmentShader=Sr(t,Nh),this.program=Rr(t,[this.vertexShader,this.fragmentShader]);var o=["u_cameraAngle","u_nodeDataTexture","u_nodeDataTextureWidth","u_frameTextureWidth","u_frameTextureHeight","u_layerAttributeTexture","u_layerAttributeTextureWidth","u_layerAttributeTexelsPerNode"].concat($(this.shapeUniforms.map(function(p){return p.name}))),s=G(o),l;try{for(s.s();!(l=s.n()).done;){var u=l.value;this.uniformLocations[u]=t.getUniformLocation(this.program,u)}}catch(p){s.e(p)}finally{s.f()}var c=e.FLOATS_PER_POINT*4;this.vao=t.createVertexArray(),this.buffer=t.createBuffer(),t.bindVertexArray(this.vao),t.bindBuffer(t.ARRAY_BUFFER,this.buffer);var d=G([["a_nodeIndex",0],["a_positionMode",4],["a_labelAngle",8]]),f;try{for(d.s();!(f=d.n()).done;){var h=Q(f.value,2),g=h[0],m=h[1],v=t.getAttribLocation(this.program,g);v>=0&&(t.enableVertexAttribArray(v),t.vertexAttribPointer(v,1,t.FLOAT,!1,c,m))}}catch(p){d.e(p)}finally{d.f()}t.bindVertexArray(null)}return Z(e,[{key:"run",value:function(n,r,a,i,o){if(r!==0){var s=this.gl;s.useProgram(this.program),s.bindVertexArray(this.vao),s.bindBuffer(s.ARRAY_BUFFER,this.buffer),s.bufferData(s.ARRAY_BUFFER,n.subarray(0,r*e.FLOATS_PER_POINT),s.DYNAMIC_DRAW);var l=this.uniformLocations;s.uniform1f(l.u_cameraAngle,i.cameraAngle),s.uniform1i(l.u_nodeDataTexture,i.nodeDataTextureUnit),s.uniform1i(l.u_nodeDataTextureWidth,i.nodeDataTextureWidth),s.uniform1f(l.u_frameTextureWidth,a.getTextureWidth()),s.uniform1f(l.u_frameTextureHeight,a.getTextureHeight()),this.hasAttributeData&&o&&(o.bind(et),l.u_layerAttributeTexture&&s.uniform1i(l.u_layerAttributeTexture,et),l.u_layerAttributeTextureWidth&&s.uniform1i(l.u_layerAttributeTextureWidth,o.getTextureWidth()),l.u_layerAttributeTexelsPerNode&&s.uniform1i(l.u_layerAttributeTexelsPerNode,o.getTexelsPerItem()));var u=G(this.shapeUniforms),c;try{for(u.s();!(c=u.n()).done;){var d=c.value;Ga(s,this.uniformLocations[d.name],d)}}catch(f){u.e(f)}finally{u.f()}a.bindAsRenderTarget(),s.disable(s.BLEND),s.disable(s.DEPTH_TEST),s.drawArrays(s.POINTS,0,r),s.bindFramebuffer(s.FRAMEBUFFER,null),s.bindVertexArray(null)}}},{key:"kill",value:function(){var n=this.gl;n.deleteProgram(this.program),n.deleteShader(this.vertexShader),n.deleteShader(this.fragmentShader),n.deleteBuffer(this.buffer),n.deleteVertexArray(this.vao)}}])})();R(Wa,"FLOATS_PER_POINT",3);function Oa(e,t,n,r,a){var i,o=r.label,s=o===void 0?{}:o,l=r.shapes;if(l.length===0)throw new Error("createNodeProgram: at least one shape must be provided in 'shapes'");var u={},c=[],d;l.forEach(function(E,S){var w=jo(E);S===0&&(d=w),u[E.name]=S,c[S]=Ut(w)});var f=$(r.layers),h=Fa({shapes:l,layers:f,shapeGlobalIds:l.length>1?c:void 0,antialias:a}),g=bs(e,null,n,{label:s}),m=fs(e,null,n,{shapes:l,layers:f,getAttributeTexture:function(){return T.getAttributeTexture()},label:s,shapeGlobalIds:l.length>1?c:void 0}),v=Lh(e,t,n,{label:s}),p=r.labelAttachments&&Object.keys(r.labelAttachments).length>0?Dh(e,null,n,{label:s}):null,b=new Wa(e,{shapes:l,layers:f,shapeGlobalIds:l.length>1?c:void 0}),y=Fr(l,f),_=ke(y),x=(i=(function(E){function S(w,A,C){var D;K(this,S),D=ae(this,S,[w,A,C]),R(D,"layerLifecycles",new Map),R(D,"layersNeedingRegeneration",new Set),R(D,"attrDescriptors",[]),D._pickingBuffer=A;var F=x.layerTextures.get(w);F||(F=new Cr(w,_),x.layerTextures.set(w,F)),D.layerAttributeTexture=F;var k=x.textureRefCounts.get(w)||0;x.textureRefCounts.set(w,k+1),D.packedAttributeData=new Float32Array(_.floatsPerItem),f.forEach(function(z,N){if(z.lifecycle){var P={gl:w,renderer:{refresh:function(){return C.refresh()}},getUniformLocation:function(j){return w.getUniformLocation(D.normalProgram.program,j)},requestShaderRegeneration:function(){D.layersNeedingRegeneration.add(N)},requestRefresh:function(){C.refresh()}},I=z.lifecycle(P);D.layerLifecycles.set(N,I)}}),D.layerLifecycles.forEach(function(z){var N;(N=z.init)===null||N===void 0||N.call(z)});var L=new Map;return D.layerLifecycles.forEach(function(z,N){z.getAttributeData&&L.set(N,z)}),D.attrDescriptors=kr(y,_,L),D}return ie(S,E),Z(S,[{key:"getDefinition",value:function(){var A=WebGL2RenderingContext,C=A.FLOAT,D=A.TRIANGLE_STRIP;return{VERTICES:4,VERTEX_SHADER_SOURCE:h.vertexShader,FRAGMENT_SHADER_SOURCE:h.fragmentShader,METHOD:D,UNIFORMS:h.uniforms,ATTRIBUTES:h.attributes,CONSTANT_ATTRIBUTES:[{name:"a_quadCorner",size:2,type:C}],CONSTANT_DATA:[[-1,-1],[1,-1],[-1,1],[1,1]]}}},{key:"maybeRegenerateShaders",value:function(){var A=this;if(this.layersNeedingRegeneration.size!==0){f=f.map(function(N,P){if(A.layersNeedingRegeneration.has(P)){var I=A.layerLifecycles.get(P);if(I!=null&&I.regenerate){var M=I.regenerate();return B(B({},M),{},{lifecycle:N.lifecycle})}}return N}),this.layersNeedingRegeneration.clear(),h=Fa({shapes:l,layers:f,shapeGlobalIds:l.length>1?c:void 0,antialias:this.renderer.getSetting("antialiasNodes")});var C=this.normalProgram.gl,D=this.normalProgram,F=D.program,k=D.buffer,L=D.vertexShader,z=D.fragmentShader;C.deleteProgram(F),C.deleteBuffer(k),C.deleteShader(L),C.deleteShader(z),this.normalProgram=this.getProgramInfo("normal",C,h.vertexShader,h.fragmentShader,this._pickingBuffer)}}},{key:"allocateNode",value:function(A){this.layerAttributeTexture.allocate(A)}},{key:"freeNode",value:function(A){this.layerAttributeTexture.free(A)}},{key:"getAttributeTexture",value:function(){return this.layerAttributeTexture}},{key:"uploadLayerTexture",value:function(){this.layerAttributeTexture.upload()}},{key:"process",value:function(A,C,D,F,k){var L=C*this.STRIDE;if(D.visibility==="hidden"){for(var z=L+this.STRIDE;L<z;L++)this.floats[L]=0;this.floats[C*this.STRIDE]=ns;return}this.processVisibleItem(ft(A),L,D,F,k)}},{key:"processVisibleItem",value:function(A,C,D,F,k){var L,z=this.floats,N=this.ints;if(z[C++]=F,N[C++]=A,z[C++]=(L=D.opacity)!==null&&L!==void 0?L:1,_.floatsPerItem!==0){var P=this.packedAttributeData;Lr(this.attrDescriptors,D,P,D.color,this.layerLifecycles,0),this.layerAttributeTexture.updateAllAttributes(k,P)}}},{key:"setUniforms",value:function(A,C){var D=this,F=C.gl,k=C.uniformLocations;k.u_matrix&&F.uniformMatrix3fv(k.u_matrix,!1,A.matrix),k.u_sizeRatio&&F.uniform1f(k.u_sizeRatio,A.sizeRatio),k.u_correctionRatio&&F.uniform1f(k.u_correctionRatio,A.correctionRatio),k.u_pickingPadding&&F.uniform1f(k.u_pickingPadding,A.nodePickingPadding),k.u_cameraAngle&&F.uniform1f(k.u_cameraAngle,A.cameraAngle),k.u_nodeDataTexture&&F.uniform1i(k.u_nodeDataTexture,A.nodeDataTextureUnit),k.u_nodeDataTextureWidth&&F.uniform1i(k.u_nodeDataTextureWidth,A.nodeDataTextureWidth),k.u_layerAttributeTexture&&(this.layerAttributeTexture.bind(et),F.uniform1i(k.u_layerAttributeTexture,et)),k.u_layerAttributeTextureWidth&&F.uniform1i(k.u_layerAttributeTextureWidth,this.layerAttributeTexture.getTextureWidth()),k.u_layerAttributeTexelsPerNode&&F.uniform1i(k.u_layerAttributeTexelsPerNode,this.layerAttributeTexture.getTexelsPerItem()),l.forEach(function(L){L.uniforms.forEach(function(z){D.setTypedUniform(z,C)})}),f.forEach(function(L){L.uniforms.forEach(function(z){D.setTypedUniform(z,C)})})}},{key:"render",value:function(A,C,D){this.maybeRegenerateShaders(),de(S,"render",this,3)([A,C,D])}},{key:"renderProgram",value:function(A,C){var D=C.gl,F=C.program;D.useProgram(F),C===this.normalProgram&&this.layerLifecycles.forEach(function(k){var L;(L=k.beforeRender)===null||L===void 0||L.call(k)}),de(S,"renderProgram",this,3)([A,C])}},{key:"kill",value:function(){this.layerLifecycles.forEach(function(D){var F;(F=D.kill)===null||F===void 0||F.call(D)}),this.layerLifecycles.clear();var A=this.normalProgram.gl,C=(x.textureRefCounts.get(A)||1)-1;C<=0?(this.layerAttributeTexture.kill(),x.layerTextures.delete(A),x.textureRefCounts.delete(A)):x.textureRefCounts.set(A,C),de(S,"kill",this,3)([])}}])})(rt),R(i,"layerTextures",new WeakMap),R(i,"textureRefCounts",new WeakMap),i),T=new x(e,null,n);return{nodeProgram:T,labelProgram:g,backdropProgram:m,labelBackgroundProgram:v,attachmentProgram:p,framePass:b,shapeSlug:d,shapeNameToIndex:l.length>1?u:void 0,shapeGlobalIds:l.length>1?c:void 0}}var tt=6;function Tn(e){var t=e.offsets,n=e.specs,r=e.floatsPerItem,a=Object.keys(t);if(a.length===0||r===0)return{uniformDeclarations:"",uniformNames:[],vertexVaryingDeclarations:"",fragmentVaryingDeclarations:"",fetchCode:"",varyingAssignments:""};var i=`
uniform sampler2D u_edgeAttributeTexture;
uniform int u_edgeAttributeTextureWidth;
uniform int u_edgeAttributeTexelsPerEdge;`,o=["u_edgeAttributeTexture","u_edgeAttributeTextureWidth","u_edgeAttributeTexelsPerEdge"],s=a.map(function(h){var g=n[h].size===1?"float":"vec".concat(n[h].size);return"".concat(g," v_").concat(h,";")}),l=s.map(function(h){return"out ".concat(h)}).join(`
`),u=s.map(function(h){return"in ".concat(h)}).join(`
`),c=Ot(e,{varPrefix:"attr",baseTexelExpr:"edgeIdx * u_edgeAttributeTexelsPerEdge",textureWidthUniform:"u_edgeAttributeTextureWidth",textureSamplerUniform:"u_edgeAttributeTexture"}),d=c.fetchCode,f=c.varyingAssignments;return{uniformDeclarations:i,uniformNames:o,vertexVaryingDeclarations:l,fragmentVaryingDeclarations:u,fetchCode:d,varyingAssignments:f}}function Mh(e){return`
float findSourceClampT_`.concat(e,`(vec2 source, float sourceSize, int sourceShapeId, float sourceRotateAlign, vec2 target, float margin) {
  float lo = 0.0, hi = 0.5;
  float nodeExtent = sourceSize * u_correctionRatio / u_sizeRatio * 2.0;
  float effectiveSize = 1.0 - u_correctionRatio / nodeExtent;

  // Counter-rotate the query point so viewport-aligned nodes (rotateAlign=0) are
  // clamped against their on-screen orientation; graph-aligned nodes skip it.
  float ca = u_cameraAngle * (1.0 - sourceRotateAlign);
  float rc = cos(ca), rs = sin(ca);
  mat2 rot = mat2(rc, -rs, rs, rc);

  for (int i = 0; i < 12; i++) {
    float mid = (lo + hi) * 0.5;
    vec2 pos = path_`).concat(e,`_position(mid, source, target);
    vec2 localPos = rot * ((pos - source) / nodeExtent);
    float sdf = querySDF(sourceShapeId, localPos, effectiveSize);
    if (sdf < 0.0) lo = mid;
    else hi = mid;
  }

  float pathLen = path_`).concat(e,`_length(source, target);
  float marginT = (margin * u_correctionRatio / u_sizeRatio) / pathLen;
  return (lo + hi) * 0.5 + marginT;
}
`)}function Gh(e){return`
float findTargetClampT_`.concat(e,`(vec2 source, vec2 target, float targetSize, int targetShapeId, float targetRotateAlign, float margin) {
  float lo = 0.5, hi = 1.0;
  float nodeExtent = targetSize * u_correctionRatio / u_sizeRatio * 2.0;
  float effectiveSize = 1.0 - u_correctionRatio / nodeExtent;

  // See findSourceClampT_ for the rotation rationale.
  float ca = u_cameraAngle * (1.0 - targetRotateAlign);
  float rc = cos(ca), rs = sin(ca);
  mat2 rot = mat2(rc, -rs, rs, rc);

  for (int i = 0; i < 12; i++) {
    float mid = (lo + hi) * 0.5;
    vec2 pos = path_`).concat(e,`_position(mid, source, target);
    vec2 localPos = rot * ((pos - target) / nodeExtent);
    float sdf = querySDF(targetShapeId, localPos, effectiveSize);
    if (sdf < 0.0) hi = mid;
    else lo = mid;
  }

  float pathLen = path_`).concat(e,`_length(source, target);
  float marginT = (margin * u_correctionRatio / u_sizeRatio) / pathLen;
  return (lo + hi) * 0.5 - marginT;
}
`)}function ys(e){return`
// Auto-generated numerical tangent (from position via finite differences)
vec2 path_`.concat(e,`_tangent(float t, vec2 source, vec2 target) {
  float epsilon = 0.001;
  float t1 = max(0.0, t - epsilon);
  float t2 = min(1.0, t + epsilon);
  vec2 p1 = path_`).concat(e,`_position(t1, source, target);
  vec2 p2 = path_`).concat(e,`_position(t2, source, target);
  return normalize(p2 - p1);
}

// Auto-generated normal (perpendicular to tangent)
vec2 path_`).concat(e,`_normal(float t, vec2 source, vec2 target) {
  vec2 tangent = path_`).concat(e,`_tangent(t, source, target);
  return vec2(-tangent.y, tangent.x);
}
`)}function Bh(e){return`
// Auto-generated path length (samples position 16 times)
float path_`.concat(e,`_length(vec2 source, vec2 target) {
  float len = 0.0;
  vec2 prev = path_`).concat(e,`_position(0.0, source, target);
  for (int i = 1; i <= 16; i++) {
    float t = float(i) / 16.0;
    vec2 curr = path_`).concat(e,`_position(t, source, target);
    len += length(curr - prev);
    prev = curr;
  }
  return len;
}
`)}function Wh(e){return`
// Auto-generated closest_t (coarse sample + ternary search)
float path_`.concat(e,`_closest_t(vec2 p, vec2 source, vec2 target) {
  // Coarse search: find best among 10 samples
  float bestT = 0.0;
  float bestDist = 1e10;
  for (int i = 0; i <= 10; i++) {
    float t = float(i) / 10.0;
    vec2 pos = path_`).concat(e,`_position(t, source, target);
    float d = length(p - pos);
    if (d < bestDist) {
      bestDist = d;
      bestT = t;
    }
  }

  // Refine with ternary search
  float lo = max(0.0, bestT - 0.1);
  float hi = min(1.0, bestT + 0.1);
  for (int i = 0; i < 10; i++) {
    float mid1 = lo + (hi - lo) / 3.0;
    float mid2 = hi - (hi - lo) / 3.0;
    float d1 = length(p - path_`).concat(e,`_position(mid1, source, target));
    float d2 = length(p - path_`).concat(e,`_position(mid2, source, target));
    if (d1 < d2) {
      hi = mid2;
    } else {
      lo = mid1;
    }
  }
  return (lo + hi) * 0.5;
}
`)}function Oh(e){return`
// Auto-generated signed distance (via closest_t + normal)
float path_`.concat(e,`_distance(vec2 p, vec2 source, vec2 target) {
  float closestT = path_`).concat(e,`_closest_t(p, source, target);
  vec2 closest = path_`).concat(e,`_position(closestT, source, target);
  vec2 diff = p - closest;
  float dist = length(diff);
  if (dist < 0.0001) return 0.0;

  // Get normal at closest point
  vec2 normal = path_`).concat(e,`_normal(closestT, source, target);
  return dist * sign(dot(diff, normal));
}
`)}function Uh(e){return`
// Auto-generated t_at_distance (binary search)
float path_`.concat(e,`_t_at_distance(float targetDist, vec2 source, vec2 target) {
  if (targetDist <= 0.0) return 0.0;

  float totalLen = path_`).concat(e,`_length(source, target);
  if (targetDist >= totalLen) return 1.0;

  // Binary search for t
  float lo = 0.0, hi = 1.0;
  for (int i = 0; i < 12; i++) {
    float mid = (lo + hi) * 0.5;

    // Compute arc length from 0 to mid
    float arcLen = 0.0;
    vec2 prev = path_`).concat(e,`_position(0.0, source, target);
    for (int j = 1; j <= 8; j++) {
      float t = mid * float(j) / 8.0;
      vec2 curr = path_`).concat(e,`_position(t, source, target);
      arcLen += length(curr - prev);
      prev = curr;
    }

    if (arcLen < targetDist) {
      lo = mid;
    } else {
      hi = mid;
    }
  }
  return (lo + hi) * 0.5;
}
`)}function br(e,t){var n=new RegExp("\\b(float|vec[234]|void|int|bool)\\s+".concat(t,"\\s*\\("));return n.test(e)}function _s(e,t){var n=[];return br(t,"path_".concat(e,"_length"))||n.push(Bh(e)),br(t,"path_".concat(e,"_closest_t"))||n.push(Wh(e)),br(t,"path_".concat(e,"_distance"))||n.push(Oh(e)),br(t,"path_".concat(e,"_t_at_distance"))||n.push(Uh(e)),n.length>0?`
// ============================================================================
// Auto-generated fallback functions (path only provided position)
// ============================================================================
`.concat(n.join(`
`)):""}function xs(e){var t,n,r,a=e.paths,i=e.layers,o=(t=e.extremities)!==null&&t!==void 0?t:[];if(a.length===0)throw new Error("At least one path is required in 'paths'");if(i.length===0)throw new Error("At least one layer is required in 'layers'");var s=(n=e.defaultHead)!==null&&n!==void 0?n:"none",l=(r=e.defaultTail)!==null&&r!==void 0?r:"none";return{paths:a,extremities:o,layers:i,path:a[0],layer:i[0],defaultHead:s,defaultTail:l}}var Ts=WebGL2RenderingContext,bn=Ts.FLOAT,Go=Ts.UNSIGNED_BYTE;function Hh(e,t,n){var r=new Set(["u_matrix","u_sizeRatio","u_correctionRatio","u_zoomRatio","u_pixelRatio","u_cameraAngle","u_minEdgeThickness","u_pickingPadding","u_nodeDataTexture","u_nodeDataTextureWidth","u_edgeDataTexture","u_edgeDataTextureWidth","u_edgeFrameTexture","u_edgeFrameTextureWidth","u_edgeAttributeTexture","u_edgeAttributeTextureWidth","u_edgeAttributeTexelsPerEdge"]),a=new Set(r);return e.forEach(function(i){return i.uniforms.forEach(function(o){return a.add(o.name)})}),t.forEach(function(i){return i.uniforms.forEach(function(o){return a.add(o.name)})}),n.forEach(function(i){return i.uniforms.forEach(function(o){return a.add(o.name)})}),Array.from(a)}function Ua(e){var t=new Set,n=[],r=G(e),a;try{for(r.s();!(a=r.n()).done;){var i=a.value;t.has(i.name)||(t.add(i.name),n.push("// Path: ".concat(i.name)),n.push(i.glsl),n.push(ys(i.name)),n.push(_s(i.name,i.glsl)))}}catch(o){r.e(o)}finally{r.f()}return n.join(`

`)}function jh(e){var t=[],n=G(e),r;try{for(n.s();!(r=n.n()).done;){var a=r.value;t.push("// Extremity: ".concat(a.name)),t.push(a.glsl)}}catch(i){n.e(i)}finally{n.f()}return t.join(`

`)}var $h=[{queryName:"queryPathPosition",pathFunc:"position",returnType:"vec2",params:"float t, vec2 source, vec2 target",args:"t, source, target"},{queryName:"queryPathTangent",pathFunc:"tangent",returnType:"vec2",params:"float t, vec2 source, vec2 target",args:"t, source, target"},{queryName:"queryPathNormal",pathFunc:"normal",returnType:"vec2",params:"float t, vec2 source, vec2 target",args:"t, source, target"},{queryName:"queryPathLength",pathFunc:"length",returnType:"float",params:"vec2 source, vec2 target",args:"source, target"},{queryName:"queryPathClosestT",pathFunc:"closest_t",returnType:"float",params:"vec2 p, vec2 source, vec2 target",args:"p, source, target"}];function Vh(e,t){var n=t.queryName,r=t.pathFunc,a=t.returnType,i=t.params,o=t.args;if(e.length===1)return"".concat(a," ").concat(n,"(int pathId, ").concat(i,`) {
  return path_`).concat(e[0].name,"_").concat(r,"(").concat(o,`);
}`);var s=e.map(function(l,u){return"    case ".concat(u,": return path_").concat(l.name,"_").concat(r,"(").concat(o,");")}).join(`
`);return"".concat(a," ").concat(n,"(int pathId, ").concat(i,`) {
  switch (pathId) {
`).concat(s,`
    default: return path_`).concat(e[0].name,"_").concat(r,"(").concat(o,`);
  }
}`)}function Ha(e){return $h.map(function(t){return Vh(e,t)}).join(`

`)}function qh(e){if(e.length===1)return`float queryExtremitySDF(int extremityId, vec2 uv, float lengthRatio, float widthRatio) {
  return extremity_`.concat(e[0].name,`(uv, lengthRatio, widthRatio);
}`);var t=e.map(function(n,r){return"    case ".concat(r,": return extremity_").concat(n.name,"(uv, lengthRatio, widthRatio);")}).join(`
`);return`float queryExtremitySDF(int extremityId, vec2 uv, float lengthRatio, float widthRatio) {
  switch (extremityId) {
`.concat(t,`
    default: return extremity_`).concat(e[0].name,`(uv, lengthRatio, widthRatio);
  }
}`)}function Xh(e){var t=[],n=G(e),r;try{for(n.s();!(r=n.n()).done;){var a=r.value;t.push(Mh(a.name)),t.push(Gh(a.name))}}catch(s){n.e(s)}finally{n.f()}if(e.length===1)t.push(`
float queryFindSourceClampT(int pathId, vec2 source, float sourceSize, int sourceShapeId, float sourceRotateAlign, vec2 target, float margin) {
  return findSourceClampT_`.concat(e[0].name,`(source, sourceSize, sourceShapeId, sourceRotateAlign, target, margin);
}

float queryFindTargetClampT(int pathId, vec2 source, vec2 target, float targetSize, int targetShapeId, float targetRotateAlign, float margin) {
  return findTargetClampT_`).concat(e[0].name,`(source, target, targetSize, targetShapeId, targetRotateAlign, margin);
}`));else{var i=e.map(function(s,l){return"    case ".concat(l,": return findSourceClampT_").concat(s.name,"(source, sourceSize, sourceShapeId, sourceRotateAlign, target, margin);")}).join(`
`),o=e.map(function(s,l){return"    case ".concat(l,": return findTargetClampT_").concat(s.name,"(source, target, targetSize, targetShapeId, targetRotateAlign, margin);")}).join(`
`);t.push(`
float queryFindSourceClampT(int pathId, vec2 source, float sourceSize, int sourceShapeId, float sourceRotateAlign, vec2 target, float margin) {
  switch (pathId) {
`.concat(i,`
    default: return findSourceClampT_`).concat(e[0].name,`(source, sourceSize, sourceShapeId, sourceRotateAlign, target, margin);
  }
}

float queryFindTargetClampT(int pathId, vec2 source, vec2 target, float targetSize, int targetShapeId, float targetRotateAlign, float margin) {
  switch (pathId) {
`).concat(o,`
    default: return findTargetClampT_`).concat(e[0].name,`(source, target, targetSize, targetShapeId, targetRotateAlign, margin);
  }
}`))}return t.join(`

`)}function Yh(e,t,n,r,a){var i=ke([].concat($(e),$(n))),o=Tn(i),s=jt(r,a),l=Object.keys(s.offsets),u=function(E){var S=s.specs[E].size;return S===1?"float":"vec".concat(S)},c=l.map(function(T){return"".concat(u(T)," v_source_").concat(T,`;
`).concat(u(T)," v_target_").concat(T,";")}).join(`
`),d=Ot(s,{varPrefix:"srcNodeAttr",baseTexelExpr:"srcIdx * u_layerAttributeTexelsPerNode",textureWidthUniform:"u_layerAttributeTextureWidth",textureSamplerUniform:"u_layerAttributeTexture",outputPrefix:"v_source_"}),f=Ot(s,{varPrefix:"tgtNodeAttr",baseTexelExpr:"tgtIdx * u_layerAttributeTexelsPerNode",textureWidthUniform:"u_layerAttributeTextureWidth",textureSamplerUniform:"u_layerAttributeTexture",outputPrefix:"v_target_"}),h=l.map(function(T){return"".concat(u(T)," g_").concat(T,";")}).join(`
`),g=l.map(function(T){return"  g_".concat(T," = v_source_").concat(T,";")}).join(`
`),m=l.map(function(T){return"  g_".concat(T," = v_target_").concat(T,";")}).join(`
`),v=new Set,p=[],b=function(E){v.has(E.name)||(v.add(E.name),p.push("uniform ".concat(E.type," ").concat(E.name,";")))};e.forEach(function(T){return T.uniforms.forEach(b)}),t.forEach(function(T){return T.uniforms.forEach(b)});var y=t.map(function(T){return re(T.widthFactor)}).join(", "),_=Math.max.apply(Math,$(e.map(function(T){return T.minBodyLengthRatio||0}))),x=`#version 300 es

// Node and edge data textures
uniform sampler2D u_nodeDataTexture;
uniform int u_nodeDataTextureWidth;
uniform sampler2D u_edgeDataTexture;
uniform int u_edgeDataTextureWidth;

// Edge attribute texture (for path-specific attributes like curvature)
`.concat(o.uniformDeclarations,`

// Node attribute texture (for node shape attributes)
uniform sampler2D u_layerAttributeTexture;
uniform int u_layerAttributeTextureWidth;
uniform int u_layerAttributeTexelsPerNode;

// Render params needed for clamping
uniform float u_sizeRatio;
uniform float u_correctionRatio;
uniform float u_cameraAngle;
uniform float u_minEdgeThickness;

// Edge-frame texture dimensions, for scattering each point to its texel
uniform float u_frameTextureWidth;
uniform float u_frameTextureHeight;

// Custom path/extremity uniforms
`).concat(p.join(`
`),`

// Path attribute varyings \u2014 assigned so path functions can read them as globals
`).concat(o.vertexVaryingDeclarations,`

// Node size varyings \u2014 read by path functions like loop
out float v_sourceNodeSize;
out float v_targetNodeSize;

// Node shape attributes per endpoint, and the ones querySDF reads
`).concat(c,`
`).concat(h,`

// Scattered output written to the edge-frame texel: [tStart, tEnd, straightenFactor, pathLength]
out vec4 v_clamp;

// Extremity width factor array (needed for extremityScale computation)
const float EXTREMITY_WIDTH_FACTORS[`).concat(t.length,"] = float[](").concat(y,`);

// Node-data fetch helpers (geometry texel + rotation-flags texel)
`).concat(Be,`
`).concat(nt,`

// Shape SDFs for node boundary clamping
`).concat(Vo(),`
`).concat(qo(new Set(l)),`

// Path functions
`).concat(Ua(e),`
`).concat(Ha(e),`
`).concat(Xh(e),`

void main() {
  // One point per edge-data row; the row index is the edge-frame texel target.
  int edgeIdx = gl_VertexID;
  int texel0Idx = edgeIdx * 2;
  int texel1Idx = edgeIdx * 2 + 1;
  ivec2 edgeTexCoord0 = ivec2(texel0Idx % u_edgeDataTextureWidth, texel0Idx / u_edgeDataTextureWidth);
  ivec2 edgeTexCoord1 = ivec2(texel1Idx % u_edgeDataTextureWidth, texel1Idx / u_edgeDataTextureWidth);
  vec4 edgeData0 = texelFetch(u_edgeDataTexture, edgeTexCoord0, 0);
  vec4 edgeData1 = texelFetch(u_edgeDataTexture, edgeTexCoord1, 0);

  int srcIdx = int(edgeData0.x);
  int tgtIdx = int(edgeData0.y);
  float a_thickness = edgeData0.z;
  float a_headLengthRatio = edgeData1.x;
  float a_tailLengthRatio = edgeData1.y;
  int pathId = int(edgeData1.z);
  int extremityPacked = int(edgeData1.w);
  int headId = extremityPacked >> 4;
  int tailId = extremityPacked & 15;

  // Fetch path/layer attributes and assign to path-function globals
`).concat(o.fetchCode,`
`).concat(o.varyingAssignments,`

  // Fetch node shape attributes for both endpoints
`).concat(d.fetchCode,`
`).concat(d.varyingAssignments,`
`).concat(f.fetchCode,`
`).concat(f.varyingAssignments,`

  // Fetch node geometry (texel 0) and rotation flags (texel 1).
  vec4 srcNodeData = readNodeData(u_nodeDataTexture, u_nodeDataTextureWidth, srcIdx);
  vec4 tgtNodeData = readNodeData(u_nodeDataTexture, u_nodeDataTextureWidth, tgtIdx);

  vec2 a_source = srcNodeData.xy;
  vec2 a_target = tgtNodeData.xy;
  float a_sourceSize = srcNodeData.z;
  float a_targetSize = tgtNodeData.z;
  float a_sourceShapeId = srcNodeData.w;
  float a_targetShapeId = tgtNodeData.w;
  // Per-node rotation alignment (0 = viewport, 1 = graph), for boundary clamping.
  float a_sourceRotateAlign = readNodeFlags(u_nodeDataTexture, u_nodeDataTextureWidth, srcIdx).r;
  float a_targetRotateAlign = readNodeFlags(u_nodeDataTexture, u_nodeDataTextureWidth, tgtIdx).r;

  v_sourceNodeSize = a_sourceSize;
  v_targetNodeSize = a_targetSize;

  float headLengthRatio = a_headLengthRatio;
  float tailLengthRatio = a_tailLengthRatio;
  float headWidthFactor = EXTREMITY_WIDTH_FACTORS[headId];
  float tailWidthFactor = EXTREMITY_WIDTH_FACTORS[tailId];
  float minBodyLengthRatio = `).concat(re(_),`;

  // Thickness in WebGL units (needed to compute clamping margin and extremity lengths)
  float pixelsThickness = max(a_thickness, u_minEdgeThickness * u_sizeRatio);
  float webGLThickness = pixelsThickness * u_correctionRatio / u_sizeRatio;

  // SDF clamping: find where the edge body meets the node boundaries. Always
  // searched (ungated) so labels get a true boundary clamp; the body re-applies
  // its extremity gating in-shader.
`).concat(g,`
  float tStart = queryFindSourceClampT(pathId, a_source, a_sourceSize, int(a_sourceShapeId), a_sourceRotateAlign, a_target, 0.0);
`).concat(m,`
  float tEnd = queryFindTargetClampT(pathId, a_source, a_target, a_targetSize, int(a_targetShapeId), a_targetRotateAlign, 0.0);

  // straightenFactor (frame .z) is consumed only by the body, which runs to the
  // node center when an extremity is absent. Derive the straightening math from
  // these gated clamps \u2014 not the ungated boundary ones above \u2014 so it matches the
  // geometry it drives. The ungated tStart/tEnd remain what labels read.
  float bodyTStart = tailLengthRatio > 0.0 ? tStart : 0.0;
  float bodyTEnd = headLengthRatio > 0.0 ? tEnd : 1.0;

  // Path length and zone boundaries (needed for straightening check)
  float pathLength = queryPathLength(pathId, a_source, a_target);
  float visibleLength = pathLength * (bodyTEnd - bodyTStart);

  float headLength = headLengthRatio * webGLThickness;
  float tailLength = tailLengthRatio * webGLThickness;
  float minBodyLength = minBodyLengthRatio * webGLThickness;

  float totalNeededLength = headLength + tailLength + minBodyLength;
  float extremityScale = 1.0;
  if (totalNeededLength > visibleLength && totalNeededLength > 0.0001) {
    extremityScale = visibleLength / totalNeededLength;
    headLength *= extremityScale;
    tailLength *= extremityScale;
  }

  float headLengthT = pathLength > 0.0001 ? headLength / pathLength : 0.0;
  float tailLengthT = pathLength > 0.0001 ? tailLength / pathLength : 0.0;

  float tTailEnd = bodyTStart + tailLengthT;
  float tHeadStart = bodyTEnd - headLengthT;
  if (tTailEnd > tHeadStart) {
    float mid = (bodyTStart + bodyTEnd) * 0.5;
    tTailEnd = mid;
    tHeadStart = mid;
  }

  // Straighten factor: blend toward straight line when path twists in extremity zones
  float straightenFactor = 0.0;
  {
    float maxDeviation = 0.0;
    if (tailLengthT > 0.0001) {
      vec2 tailTang = queryPathTangent(pathId, tTailEnd, a_source, a_target);
      vec2 tailChord = queryPathPosition(pathId, bodyTStart, a_source, a_target)
                     - queryPathPosition(pathId, tTailEnd, a_source, a_target);
      float tailChordLen = length(tailChord);
      if (tailChordLen > 0.0001) {
        maxDeviation = max(maxDeviation, 1.0 - dot(-tailTang, tailChord / tailChordLen));
      }
    }
    if (headLengthT > 0.0001) {
      vec2 headTang = queryPathTangent(pathId, tHeadStart, a_source, a_target);
      vec2 headChord = queryPathPosition(pathId, bodyTEnd, a_source, a_target)
                     - queryPathPosition(pathId, tHeadStart, a_source, a_target);
      float headChordLen = length(headChord);
      if (headChordLen > 0.0001) {
        maxDeviation = max(maxDeviation, 1.0 - dot(headTang, headChord / headChordLen));
      }
    }
    straightenFactor = smoothstep(0.035, 0.5, maxDeviation);
  }

  // When straightening, blend the ungated tStart/tEnd (the label-facing clamps)
  // toward straight-line clamp positions.
  if (straightenFactor > 0.001) {
    if (tailLengthRatio > 0.0) {
`).concat(g,`
      float srcExtent = a_sourceSize * u_correctionRatio / u_sizeRatio * 2.0;
      float srcEffective = 1.0 - u_correctionRatio / srcExtent;
      float srcCa = u_cameraAngle * (1.0 - a_sourceRotateAlign);
      mat2 srcRot = mat2(cos(srcCa), -sin(srcCa), sin(srcCa), cos(srcCa));
      float lo = 0.0, hi = 0.5;
      for (int i = 0; i < 12; i++) {
        float mid = (lo + hi) * 0.5;
        vec2 pos = mix(a_source, a_target, mid);
        vec2 localPos = srcRot * ((pos - a_source) / srcExtent);
        float sdf = querySDF(int(a_sourceShapeId), localPos, srcEffective);
        if (sdf < 0.0) lo = mid; else hi = mid;
      }
      tStart = mix(tStart, (lo + hi) * 0.5, straightenFactor);
    }
    if (headLengthRatio > 0.0) {
`).concat(m,`
      float tgtExtent = a_targetSize * u_correctionRatio / u_sizeRatio * 2.0;
      float tgtEffective = 1.0 - u_correctionRatio / tgtExtent;
      float tgtCa = u_cameraAngle * (1.0 - a_targetRotateAlign);
      mat2 tgtRot = mat2(cos(tgtCa), -sin(tgtCa), sin(tgtCa), cos(tgtCa));
      float lo = 0.5, hi = 1.0;
      for (int i = 0; i < 12; i++) {
        float mid = (lo + hi) * 0.5;
        vec2 pos = mix(a_source, a_target, mid);
        vec2 localPos = tgtRot * ((pos - a_target) / tgtExtent);
        float sdf = querySDF(int(a_targetShapeId), localPos, tgtEffective);
        if (sdf < 0.0) hi = mid; else lo = mid;
      }
      tEnd = mix(tEnd, (lo + hi) * 0.5, straightenFactor);
    }
  }

  v_clamp = vec4(tStart, tEnd, straightenFactor, pathLength);

  // Scatter this point to its edge's texel center in the frame texture.
  float x = mod(float(edgeIdx), u_frameTextureWidth);
  float y = floor(float(edgeIdx) / u_frameTextureWidth);
  vec2 ndc = (vec2(x, y) + 0.5) / vec2(u_frameTextureWidth, u_frameTextureHeight) * 2.0 - 1.0;
  gl_Position = vec4(ndc, 0.0, 1.0);
  gl_PointSize = 1.0;
}
`);return x}var yr=0,Bo=1,_r=2;function Kh(e,t,n){var r=[];n&&(r.push([yr,0,-1],[yr,0,1]),r.push([yr,1,-1],[yr,1,1]));for(var a=0;a<=e;a++){var i=a/e;r.push([Bo,i,-1],[Bo,i,1])}return t&&(r.push([_r,0,-1],[_r,0,1]),r.push([_r,1,-1],[_r,1,1])),{data:r,attributes:[{name:"a_zone",size:1,type:bn},{name:"a_zoneT",size:1,type:bn},{name:"a_side",size:1,type:bn}],verticesPerEdge:r.length}}function Zh(e,t,n,r){var a=ke([].concat($(e),$(n))),i=Tn(a),o=new Set(["u_matrix","u_sizeRatio","u_correctionRatio","u_zoomRatio","u_pixelRatio","u_cameraAngle","u_minEdgeThickness","u_pickingPadding","u_nodeDataTexture"]),s=new Set,l=[],u=function(p){!o.has(p.name)&&!s.has(p.name)&&(s.add(p.name),l.push("uniform ".concat(p.type," ").concat(p.name,";")))};e.forEach(function(v){return v.uniforms.forEach(u)}),t.forEach(function(v){return v.uniforms.forEach(u)}),n.forEach(function(v){return v.uniforms.forEach(u)});var c=r.map(function(v){var p=v.size===1?"float":"vec".concat(v.size);return"in ".concat(p," ").concat(v.name,";")}).join(`
`),d=t.map(function(v){return re(v.widthFactor)}).join(", "),f=Math.max.apply(Math,$(e.map(function(v){return v.minBodyLengthRatio||0}))),h=n.some(function(v){return v.needsNodeColors}),g=e.some(function(v){return v.needsNodeSize}),m=`#version 300 es

// Constant attributes (per vertex)
`.concat(c,`

// Per-edge attributes
// Edge data (source/target indices, thickness, extremity ratios, path/extremity IDs)
// is fetched from edge data texture via edge index
in float a_edgeIndex;   // Index into edge data texture
in vec4 a_color;        // Edge color
in vec4 a_id;           // Edge ID for picking
in float a_opacity;     // Edge opacity

// Standard uniforms
uniform mat3 u_matrix;
uniform float u_sizeRatio;
uniform float u_correctionRatio;
uniform float u_zoomRatio;
uniform float u_pixelRatio;
uniform float u_cameraAngle;
uniform float u_minEdgeThickness;
#ifdef PICKING_MODE
uniform float u_pickingPadding;
#endif
uniform sampler2D u_nodeDataTexture;
uniform int u_nodeDataTextureWidth;
uniform sampler2D u_edgeDataTexture;
uniform int u_edgeDataTextureWidth;
uniform sampler2D u_edgeFrameTexture;
uniform int u_edgeFrameTextureWidth;

// Edge path attribute texture uniforms
`).concat(i.uniformDeclarations,`

// Custom uniforms
`).concat(l.join(`
`),`

// Standard varyings
out vec4 v_color;
out float v_opacity;
out vec4 v_id;
out float v_thickness;       // Edge body thickness (in consistent units)
out float v_t;
out float v_tStart;
out float v_tEnd;
out float v_side;
out float v_antialiasingWidth;  // Anti-aliasing width (normalized: u_correctionRatio / thickness)
out vec2 v_source;
out vec2 v_target;
out float v_edgeLength;
`).concat(g?`
out float v_sourceNodeSize;  // Source node size (mirrored in labels/generator.ts as plain float)
out float v_targetNodeSize;  // Target node size (mirrored in labels/generator.ts as plain float)`:"",`
`).concat(h?`
out vec4 v_sourceColor;
out vec4 v_targetColor;`:"",`
// Zone varyings
out float v_zone;            // 0=tail, 1=body, 2=head
out float v_zoneT;           // Position within zone [0,1]
out float v_headLengthRatio; // Head length as ratio of thickness
out float v_tailLengthRatio; // Tail length as ratio of thickness
out float v_headWidthRatio;  // Head width factor
out float v_tailWidthRatio;  // Tail width factor

// Multi-path/extremity varyings
flat out int v_pathId;
flat out int v_headId;
flat out int v_tailId;

// Path/layer attribute varyings (fetched from edge attribute texture)
`).concat(i.vertexVaryingDeclarations,`

const float bias = 255.0 / 254.0;

// Width factor array for extremities (shared pool for head/tail)
const float EXTREMITY_WIDTH_FACTORS[`).concat(t.length,"] = float[](").concat(d,`);

// All path functions
`).concat(Ua(e),`

// Path selector functions
`).concat(Ha(e),`

// Node-data fetch helper (geometry texel of the two-texel node stride)
`).concat(Be,`
`).concat(h?es:"",`
// Per-edge clamp from the frame-pass (tStart, tEnd, straightenFactor, pathLength)
`).concat(yt,`

void main() {
  // Fetch edge data from edge texture (2 texels per edge)
  // Texel 0: sourceNodeIndex, targetNodeIndex, thickness, reserved
  // Texel 1: headLengthRatio, tailLengthRatio, pathId, (headId << 4) | tailId
  int edgeIdx = int(a_edgeIndex);

  // Hidden edges are flagged with a negative row by EdgeProgram.process(). Push
  // the whole primitive outside the clip volume: every vertex lands at the same
  // out-of-range position, so it is fully clipped and rasterizes nothing \u2014
  // neither to the frame buffer nor to the picking buffer.
  if (edgeIdx < 0) {
    gl_Position = vec4(2.0, 0.0, 0.0, 1.0);
    v_color = vec4(0.0);
    v_id = vec4(0.0);
    return;
  }

  int texel0Idx = edgeIdx * 2;
  int texel1Idx = edgeIdx * 2 + 1;
  ivec2 edgeTexCoord0 = ivec2(texel0Idx % u_edgeDataTextureWidth, texel0Idx / u_edgeDataTextureWidth);
  ivec2 edgeTexCoord1 = ivec2(texel1Idx % u_edgeDataTextureWidth, texel1Idx / u_edgeDataTextureWidth);
  vec4 edgeData0 = texelFetch(u_edgeDataTexture, edgeTexCoord0, 0);
  vec4 edgeData1 = texelFetch(u_edgeDataTexture, edgeTexCoord1, 0);

  // Unpack edge data
  int srcIdx = int(edgeData0.x);
  int tgtIdx = int(edgeData0.y);
  float a_thickness = edgeData0.z;
  // edgeData0.w is now reserved (curvature moved to path attribute texture)
  float a_headLengthRatio = edgeData1.x;
  float a_tailLengthRatio = edgeData1.y;
  int pathId = int(edgeData1.z);
  int extremityPacked = int(edgeData1.w);
  int headId = extremityPacked >> 4;
  int tailId = extremityPacked & 15;

  // Fetch path/layer attributes from edge attribute texture
`).concat(i.fetchCode,`

  // Assign path/layer attribute varyings
`).concat(i.varyingAssignments,`

  // Fetch source and target node geometry (texel 0 of the two-texel node stride)
  vec4 srcNodeData = readNodeData(u_nodeDataTexture, u_nodeDataTextureWidth, srcIdx);
  vec4 tgtNodeData = readNodeData(u_nodeDataTexture, u_nodeDataTextureWidth, tgtIdx);

  vec2 a_source = srcNodeData.xy;
  vec2 a_target = tgtNodeData.xy;
`).concat(g?`
  // Assign node size varyings early (path functions like loops need them during clamping)
  v_sourceNodeSize = srcNodeData.z;
  v_targetNodeSize = tgtNodeData.z;`:"",`
`).concat(h?`
  v_sourceColor = readNodeColor(u_nodeDataTexture, u_nodeDataTextureWidth, srcIdx);
  v_targetColor = readNodeColor(u_nodeDataTexture, u_nodeDataTextureWidth, tgtIdx);`:"",`

  // Convert thickness to WebGL units
  float minThickness = u_minEdgeThickness;
  float pixelsThickness = max(a_thickness, minThickness * u_sizeRatio);
  float webGLThickness = pixelsThickness * u_correctionRatio / u_sizeRatio;

  // Extremity parameters from ID lookups (shared pool)
  float headLengthRatio = a_headLengthRatio;
  float tailLengthRatio = a_tailLengthRatio;
  float headWidthFactor = EXTREMITY_WIDTH_FACTORS[headId];
  float tailWidthFactor = EXTREMITY_WIDTH_FACTORS[tailId];
  float minBodyLengthRatio = `).concat(re(f),`;

  // Per-edge values from the frame-pass texture (read by edge index).
  // tStart/tEnd are the ungated boundary clamps: re-apply the body's extremity
  // gating here (no extremity \u2192 body runs to the node center, 0/1).
  vec4 frameClamp = readFrameTexel(u_edgeFrameTexture, u_edgeFrameTextureWidth, edgeIdx);
  float tStart           = a_tailLengthRatio > 0.0 ? frameClamp.x : 0.0;
  float tEnd             = a_headLengthRatio > 0.0 ? frameClamp.y : 1.0;
  float straightenFactor = frameClamp.z;
  float pathLength       = frameClamp.w;

  // Anti-aliasing width (~1 pixel, normalized by thickness)
  float antialiasingWidth = u_correctionRatio / webGLThickness;

  float visibleLength = pathLength * (tEnd - tStart);

  // Compute extremity lengths in world units
  float headLength = headLengthRatio * webGLThickness;
  float tailLength = tailLengthRatio * webGLThickness;
  float minBodyLength = minBodyLengthRatio * webGLThickness;

  // Handle short edges: scale down extremities if needed
  float totalNeededLength = headLength + tailLength + minBodyLength;
  float extremityScale = 1.0;
  if (totalNeededLength > visibleLength && totalNeededLength > 0.0001) {
    extremityScale = visibleLength / totalNeededLength;
    headLength *= extremityScale;
    tailLength *= extremityScale;
  }

  // Convert lengths to t-values
  float headLengthT = pathLength > 0.0001 ? headLength / pathLength : 0.0;
  float tailLengthT = pathLength > 0.0001 ? tailLength / pathLength : 0.0;

  // Zone boundaries in t-space
  float tTailEnd = tStart + tailLengthT;
  float tHeadStart = tEnd - headLengthT;

  // Ensure body has non-negative length
  if (tTailEnd > tHeadStart) {
    float mid = (tStart + tEnd) * 0.5;
    tTailEnd = mid;
    tHeadStart = mid;
  }

  // Convert to webGL units for geometry expansion
  float aaWidthWebGL = antialiasingWidth * webGLThickness;

  // Extra geometry width for picking padding (0 in visual mode)
  #ifdef PICKING_MODE
    float pickingPaddingWebGL = u_pickingPadding * u_correctionRatio;
  #else
    float pickingPaddingWebGL = 0.0;
  #endif

  // Straight-line direction and normal (for blending when straightenFactor > 0)
  vec2 straightDir = length(a_target - a_source) > 0.0001
    ? normalize(a_target - a_source) : vec2(1.0, 0.0);
  vec2 straightNormal = vec2(-straightDir.y, straightDir.x);

  // Zone-based vertex processing using path selectors
  vec2 position;
  vec2 normal;
  float t;
  float zone = a_zone;
  float zoneT = a_zoneT;
  float side = a_side;

  // Scaled extremity width factors (geometry must be at least as wide as body)
  float scaledTailWidth = max(tailWidthFactor * extremityScale, 1.0);
  float scaledHeadWidth = max(headWidthFactor * extremityScale, 1.0);

  if (zone < 0.5) {
    // TAIL ZONE: rectangular quad with scaled width
    vec2 tang = queryPathTangent(pathId, tTailEnd, a_source, a_target);
    normal = vec2(-tang.y, tang.x);
    vec2 centerPos = mix(queryPathPosition(pathId, tStart, a_source, a_target),
                         queryPathPosition(pathId, tTailEnd, a_source, a_target), zoneT);
    float halfWidth = webGLThickness * scaledTailWidth * 0.5 + aaWidthWebGL + pickingPaddingWebGL;
    position = centerPos + normal * side * halfWidth;
    t = mix(tStart, tTailEnd, zoneT);

  } else if (zone < 1.5) {
    // BODY ZONE: follows path curvature with width = 1.0
    t = mix(tTailEnd, tHeadStart, zoneT);
    normal = queryPathNormal(pathId, t, a_source, a_target);
    float halfWidth = webGLThickness * 0.5 + aaWidthWebGL + pickingPaddingWebGL;
    position = queryPathPosition(pathId, t, a_source, a_target) + normal * side * halfWidth;

  } else {
    // HEAD ZONE: rectangular quad with scaled width
    vec2 tang = queryPathTangent(pathId, tHeadStart, a_source, a_target);
    normal = vec2(-tang.y, tang.x);
    vec2 centerPos = mix(queryPathPosition(pathId, tHeadStart, a_source, a_target),
                         queryPathPosition(pathId, tEnd, a_source, a_target), zoneT);
    float halfWidth = webGLThickness * scaledHeadWidth * 0.5 + aaWidthWebGL + pickingPaddingWebGL;
    position = centerPos + normal * side * halfWidth;
    t = mix(tHeadStart, tEnd, zoneT);
  }

  // Blend toward straight line based on path twist in extremity zones
  if (straightenFactor > 0.001) {
    float zoneWidth = zone < 0.5 ? webGLThickness * scaledTailWidth * 0.5 + aaWidthWebGL + pickingPaddingWebGL :
                      zone < 1.5 ? webGLThickness * 0.5 + aaWidthWebGL + pickingPaddingWebGL :
                      webGLThickness * scaledHeadWidth * 0.5 + aaWidthWebGL + pickingPaddingWebGL;
    vec2 straightPos = mix(a_source, a_target, t) + straightNormal * side * zoneWidth;
    position = mix(position, straightPos, straightenFactor);
  }

  gl_Position = vec4((u_matrix * vec3(position, 1.0)).xy, 0.0, 1.0);

  // Pass varyings to fragment shader
  v_color = a_color;
  v_color.a *= bias;
  v_opacity = a_opacity;
  v_id = a_id;
  v_thickness = webGLThickness;
  v_t = t;
  v_tStart = tStart;
  v_tEnd = tEnd;
  v_side = side;
  v_antialiasingWidth = antialiasingWidth;
  v_source = a_source;
  v_target = a_target;
  v_edgeLength = pathLength;

  // Zone varyings
  v_zone = zone;
  v_zoneT = zoneT;
  v_headLengthRatio = headLengthRatio * extremityScale;
  v_tailLengthRatio = tailLengthRatio * extremityScale;
  // Scale extremity width proportionally with length when crushed
  v_headWidthRatio = headWidthFactor * extremityScale;
  v_tailWidthRatio = tailWidthFactor * extremityScale;

  // Multi-path varyings
  v_pathId = pathId;
  v_headId = headId;
  v_tailId = tailId;
}
`);return m}function Jh(e,t,n,r){var a=ke([].concat($(e),$(n))),i=Tn(a),o=new Set(["u_matrix","u_sizeRatio","u_correctionRatio","u_zoomRatio","u_pixelRatio","u_cameraAngle","u_minEdgeThickness","u_pickingPadding"]),s=new Set,l=[],u=function(b){!o.has(b.name)&&!s.has(b.name)&&(s.add(b.name),l.push("uniform ".concat(b.type," ").concat(b.name,";")))};e.forEach(function(p){return p.uniforms.forEach(u)}),t.forEach(function(p){return p.uniforms.forEach(u)}),n.forEach(function(p){return p.uniforms.forEach(u)});var c=t.map(function(p){var b;return re((b=p.baseRatio)!==null&&b!==void 0?b:.5)}).join(", "),d=function(b){var y,_;return t.length>1?"EXTREMITY_BASE_RATIOS[".concat(b,"]"):re((y=(_=t[0])===null||_===void 0?void 0:_.baseRatio)!==null&&y!==void 0?y:.5)},f=t.some(function(p){return ze(p.length)?!0:p.length>0}),h=n.some(function(p){return p.needsNodeColors}),g=e.some(function(p){return p.needsNodeSize}),m=n.map(function(p,b){return"  // Layer ".concat(b+1,": ").concat(p.name,`
  color = blendOver(color, layer_`).concat(p.name,"(context));")}).join(`

`),v=`#version 300 es
precision highp float;

// Standard varyings
in vec4 v_color;
in float v_opacity;
in vec4 v_id;
in float v_thickness;       // Edge body thickness
in float v_t;
in float v_tStart;
in float v_tEnd;
in float v_side;
in float v_antialiasingWidth;  // Anti-aliasing width (normalized: u_correctionRatio / thickness)
in vec2 v_source;
in vec2 v_target;
in float v_edgeLength;
`.concat(g?`
in float v_sourceNodeSize;   // Source node size (mirrored in labels/generator.ts as plain float)
in float v_targetNodeSize;   // Target node size (mirrored in labels/generator.ts as plain float)`:"",`
`).concat(h?`
in vec4 v_sourceColor;
in vec4 v_targetColor;`:"",`
// Zone varyings
in float v_zone;            // 0=tail, 1=body, 2=head
in float v_zoneT;           // Position within zone [0,1]
in float v_headLengthRatio; // Head length as ratio of thickness (scaled for short edges)
in float v_tailLengthRatio; // Tail length as ratio of thickness (scaled for short edges)
in float v_headWidthRatio;  // Head width factor
in float v_tailWidthRatio;  // Tail width factor

// Multi-path/extremity varyings
flat in int v_pathId;
flat in int v_headId;
flat in int v_tailId;

// Path/layer attribute varyings (from vertex shader texture fetch)
`).concat(i.fragmentVaryingDeclarations,`

// Standard uniforms (needed by some path types)
uniform float u_sizeRatio;
uniform float u_correctionRatio;
uniform float u_cameraAngle;
#ifdef PICKING_MODE
uniform float u_pickingPadding;
#endif

// Custom uniforms
`).concat(l.join(`
`),`

// Fragment output (single target - picking handled via separate pass)
out vec4 fragColor;

`).concat(t.length>1?`// Base ratio array for extremities (shared pool for head/tail)
const float EXTREMITY_BASE_RATIOS[`.concat(t.length,"] = float[](").concat(c,");"):"",`

// EdgeContext struct
struct EdgeContext {
  float t;                   // Position along path [0, 1]
  float sdf;                 // Signed distance from centerline
  vec2 position;             // World position
  vec2 tangent;              // Path tangent
  vec2 normal;               // Path normal
  float thickness;           // Edge thickness
  float aaWidth;             // Anti-aliasing width
  float edgeLength;          // Total path length
  float tStart;              // Clamped start t
  float tEnd;                // Clamped end t
  float distanceFromSource;  // Arc distance from source
  float distanceToTarget;    // Arc distance to target
};

EdgeContext context;

// Alpha "over" compositing for layer blending
vec4 blendOver(vec4 bg, vec4 fg) {
  float a = fg.a;
  return vec4(mix(bg.rgb, fg.rgb, a), bg.a + a * (1.0 - bg.a));
}

// All path functions
`).concat(Ua(e),`

// Path selector functions
`).concat(Ha(e),`

// All extremity functions
`).concat(jh(t),`

// Extremity SDF selector (shared pool for head/tail)
`).concat(qh(t),`

// Layer functions
`).concat(n.map(function(p){return p.glsl}).join(`

`),`

// Helper: Compute arc length using path selector
float computeArcLengthMulti(int pathId, float t0, float t1, vec2 source, vec2 target, int samples) {
  float arcLen = 0.0;
  vec2 prev = queryPathPosition(pathId, t0, source, target);
  for (int i = 1; i <= samples; i++) {
    float t = t0 + (t1 - t0) * float(i) / float(samples);
    vec2 curr = queryPathPosition(pathId, t, source, target);
    arcLen += length(curr - prev);
    prev = curr;
  }
  return arcLen;
}

void main() {
  // Compute normalized t within visible edge (0 = start, 1 = end)
  float tNorm = (v_t - v_tStart) / max(v_tEnd - v_tStart, 0.0001);

  // Edge body half-thickness
  float halfThickness = v_thickness * 0.5;

  // Convert normalized AA width to webGL units (~1 pixel)
  float aaWidthWebGL = v_antialiasingWidth * v_thickness;

  // Distance from centerline based on v_side interpolation
  float zoneWidthFactor = v_zone < 0.5 ? v_tailWidthRatio :
                          v_zone < 1.5 ? 1.0 :
                          v_headWidthRatio;
  // In PICKING_MODE, inflate halfGeometryWidth to match the inflated vertex geometry
  #ifdef PICKING_MODE
    float halfGeometryWidth = halfThickness * zoneWidthFactor + aaWidthWebGL + u_pickingPadding * aaWidthWebGL;
  #else
    float halfGeometryWidth = halfThickness * zoneWidthFactor + aaWidthWebGL;
  #endif
  float distFromCenter = abs(v_side) * halfGeometryWidth;

  // Populate EdgeContext (for layer functions)
  context.t = tNorm;
  context.sdf = distFromCenter - halfThickness;
  context.position = queryPathPosition(v_pathId, v_t, v_source, v_target);
  context.tangent = queryPathTangent(v_pathId, v_t, v_source, v_target);
  context.normal = vec2(-context.tangent.y, context.tangent.x);
  context.thickness = v_thickness;
  context.aaWidth = aaWidthWebGL;
  context.edgeLength = v_edgeLength;
  context.tStart = v_tStart;
  context.tEnd = v_tEnd;

  // Compute arc distances
  float visibleLength = v_edgeLength * (v_tEnd - v_tStart);
  float pathT = v_t;
  float pathTNorm = tNorm;
`).concat(e.every(function(p){return p.linearParameterization})?`  // All paths have linear parameterization: t maps directly to arc distance
  context.distanceFromSource = pathTNorm * visibleLength;
  context.distanceToTarget = (1.0 - pathTNorm) * visibleLength;`:e.every(function(p){return!p.linearParameterization})?`  // No paths have linear parameterization: use numerical integration
  context.distanceFromSource = computeArcLengthMulti(v_pathId, v_tStart, pathT, v_source, v_target, 16);
  context.distanceToTarget = computeArcLengthMulti(v_pathId, pathT, v_tEnd, v_source, v_target, 16);`:`  // Mixed parameterization: use analytical for linear paths, numerical for others
  if (`.concat(e.filter(function(p){return p.linearParameterization}).map(function(p){return"v_pathId == ".concat(e.indexOf(p))}).join(" || "),`) {
    context.distanceFromSource = pathTNorm * visibleLength;
    context.distanceToTarget = (1.0 - pathTNorm) * visibleLength;
  } else {
    context.distanceFromSource = computeArcLengthMulti(v_pathId, v_tStart, pathT, v_source, v_target, 16);
    context.distanceToTarget = computeArcLengthMulti(v_pathId, pathT, v_tEnd, v_source, v_target, 16);
  }`),`

  // Compute SDF based on zone using extremity selector (shared pool)
  float bodySDF = distFromCenter - halfThickness;
  float finalSDF;

`).concat(f?`  // Base ratios, from baseRatioLookup
  float headBaseRatio = `.concat(d("v_headId"),`;
  float tailBaseRatio = `).concat(d("v_tailId"),`;

  if (v_zone < 0.5) {
    // TAIL ZONE: v_zoneT goes 0 (tip) to 1 (base)
    vec2 uv = vec2((1.0 - v_zoneT) * v_tailLengthRatio, v_side * v_tailWidthRatio * 0.5);
    float tailSDF = queryExtremitySDF(v_tailId, uv, v_tailLengthRatio, v_tailWidthRatio) * v_thickness;

    // Apply union only near base (v_zoneT > 1 - baseRatio)
    if (v_zoneT > 1.0 - tailBaseRatio) {
      finalSDF = min(tailSDF, bodySDF);
    } else {
      finalSDF = tailSDF;
    }
  } else if (v_zone < 1.5) {
    // BODY ZONE: distance from centerline
    finalSDF = bodySDF;
  } else {
    // HEAD ZONE: v_zoneT goes 0 (base) to 1 (tip)
    vec2 uv = vec2(v_zoneT * v_headLengthRatio, v_side * v_headWidthRatio * 0.5);
    float headSDF = queryExtremitySDF(v_headId, uv, v_headLengthRatio, v_headWidthRatio) * v_thickness;

    // Apply union only near base (v_zoneT < baseRatio)
    if (v_zoneT < headBaseRatio) {
      finalSDF = min(headSDF, bodySDF);
    } else {
      finalSDF = headSDF;
    }
  }`):`  // No extremity draws tail/head geometry, so every vertex is body zone.
  finalSDF = bodySDF;`,`

  #ifdef PICKING_MODE
    // Picking pass: output edge ID for pixels within the picking area
    if (finalSDF > u_pickingPadding * aaWidthWebGL) discard;
    fragColor = v_id;
  #else
`).concat(r?`    // Visual pass: anti-aliased edge with layers, edge opacity applied once
    float alpha = smoothstep(aaWidthWebGL, -aaWidthWebGL, finalSDF) * v_opacity;`:`    // Visual pass: hard-edged (no anti-aliasing gradient) edge with layers, edge opacity applied once
    float alpha = (finalSDF < 0.0 ? 1.0 : 0.0) * v_opacity;`,`
    if (alpha < 0.01) discard;

    // Apply layers sequentially with "over" compositing
    vec4 color = vec4(0.0);

`).concat(m,`

    // Mix with transparent to fade both color AND alpha (pre-multiplied alpha for correct blending)
    fragColor = mix(vec4(0.0), color, alpha);
  #endif
}
`);return v}function Qh(e,t,n){var r=ze(t.length)?!0:t.length>0,a=ze(n.length)?!0:n.length>0;if(e.generateConstantData){var i=e.generateConstantData();return{data:i.data,attributes:i.attributes,verticesPerEdge:i.verticesPerEdge}}return Kh(e.segments,r,a)}function Pa(e){var t,n=xs(e),r=n.paths,a=n.extremities,i=n.layers,o=(t=e.antialias)!==null&&t!==void 0?t:!0,s=new Map,l=new Map,u=new Map,c=G(r),d;try{for(c.s();!(d=c.n()).done;){var f=d.value,h=G(a),g;try{for(h.s();!(g=h.n()).done;){var m=g.value,v=G(a),p;try{for(v.s();!(p=v.n()).done;){var b=p.value,y="".concat(f.name,":").concat(m.name,":").concat(b.name),_=Qh(f,m,b);s.set(y,_.verticesPerEdge),l.set(y,_.data);var x=G(_.attributes),T;try{for(x.s();!(T=x.n()).done;){var E=T.value;u.has(E.name)||u.set(E.name,E)}}catch(q){x.e(q)}finally{x.f()}}}catch(q){v.e(q)}finally{v.f()}}}catch(q){h.e(q)}finally{h.f()}}}catch(q){c.e(q)}finally{c.f()}var S=Array.from(u.values()),w={};S.forEach(function(q,X){w[q.name]=X});var A=0,C="",D=G(s),F;try{for(D.s();!(F=D.n()).done;){var k=Q(F.value,2),L=k[0],z=k[1];z>A&&(A=z,C=L)}}catch(q){D.e(q)}finally{D.f()}var N=l.get(C)||[],P=C.split(":"),I=Q(P,1),M=I[0],j=r.find(function(q){return q.name===M}),U=[];j!=null&&j.generateConstantData?U=j.generateConstantData().attributes:U=[{name:"a_zone"},{name:"a_zoneT"},{name:"a_side"}];var Y=N.map(function(q){var X=new Array(S.length).fill(0);return U.forEach(function(Le,ye){var Ve=w[Le.name];Ve!==void 0&&ye<q.length&&(X[Ve]=q[ye])}),X});return{vertexShader:Zh(r,a,i,S),fragmentShader:Jh(r,a,i,o),uniforms:Hh(r,a,i),attributes:ef(),verticesPerEdge:A,constantData:Y,constantAttributes:S,vertexCountsPerCombination:s,constantDataPerCombination:l}}function ef(e,t,n){return[{name:"a_edgeIndex",size:1,type:bn},{name:"a_color",size:4,type:Go,normalized:!0},{name:"a_id",size:4,type:Go,normalized:!0},{name:"a_opacity",size:1,type:bn}]}var tf=`#version 300 es
precision highp float;

in vec4 v_clamp;

// RGBA32F target: stores (tStart, tEnd, straightenFactor, pathLength).
out vec4 fragColor;

void main() {
  fragColor = v_clamp;
}
`,Es=(function(){function e(t,n){K(this,e),R(this,"uniformLocations",{});var r=n.paths,a=n.extremities,i=n.layers,o=n.nodeShapes,s=n.nodeLayers;this.gl=t,this.hasAttributeData=ke([].concat($(r),$(i))).floatsPerItem>0,this.hasNodeAttributeData=Object.keys(jt(o,s).offsets).length>0,this.vertexShader=Er(t,Yh(r,a,i,o,s)),this.fragmentShader=Sr(t,tf),this.program=Rr(t,[this.vertexShader,this.fragmentShader]);var l=new Set;this.customUniforms=[];for(var u=0,c=[].concat($(r),$(a));u<c.length;u++){var d=c[u],f=G(d.uniforms),h;try{for(f.s();!(h=f.n()).done;){var g=h.value;l.has(g.name)||(l.add(g.name),this.customUniforms.push(g))}}catch(y){f.e(y)}finally{f.f()}}var m=["u_sizeRatio","u_correctionRatio","u_cameraAngle","u_minEdgeThickness","u_nodeDataTexture","u_nodeDataTextureWidth","u_edgeDataTexture","u_edgeDataTextureWidth","u_edgeAttributeTexture","u_edgeAttributeTextureWidth","u_edgeAttributeTexelsPerEdge","u_layerAttributeTexture","u_layerAttributeTextureWidth","u_layerAttributeTexelsPerNode","u_frameTextureWidth","u_frameTextureHeight"].concat($(this.customUniforms.map(function(y){return y.name}))),v=G(m),p;try{for(v.s();!(p=v.n()).done;){var b=p.value;this.uniformLocations[b]=t.getUniformLocation(this.program,b)}}catch(y){v.e(y)}finally{v.f()}this.vao=t.createVertexArray()}return Z(e,[{key:"run",value:function(n,r,a,i,o){if(a!==0){var s=this.gl,l=this.uniformLocations;s.useProgram(this.program),s.bindVertexArray(this.vao),l.u_sizeRatio&&s.uniform1f(l.u_sizeRatio,n.sizeRatio),l.u_correctionRatio&&s.uniform1f(l.u_correctionRatio,n.correctionRatio),l.u_cameraAngle&&s.uniform1f(l.u_cameraAngle,n.cameraAngle),l.u_minEdgeThickness&&s.uniform1f(l.u_minEdgeThickness,n.minEdgeThickness),l.u_nodeDataTexture&&s.uniform1i(l.u_nodeDataTexture,n.nodeDataTextureUnit),l.u_nodeDataTextureWidth&&s.uniform1i(l.u_nodeDataTextureWidth,n.nodeDataTextureWidth),l.u_edgeDataTexture&&s.uniform1i(l.u_edgeDataTexture,n.edgeDataTextureUnit),l.u_edgeDataTextureWidth&&s.uniform1i(l.u_edgeDataTextureWidth,n.edgeDataTextureWidth),l.u_frameTextureWidth&&s.uniform1f(l.u_frameTextureWidth,r.getTextureWidth()),l.u_frameTextureHeight&&s.uniform1f(l.u_frameTextureHeight,r.getTextureHeight()),this.hasAttributeData&&i&&(i.bind(tt),l.u_edgeAttributeTexture&&s.uniform1i(l.u_edgeAttributeTexture,tt),l.u_edgeAttributeTextureWidth&&s.uniform1i(l.u_edgeAttributeTextureWidth,i.getTextureWidth()),l.u_edgeAttributeTexelsPerEdge&&s.uniform1i(l.u_edgeAttributeTexelsPerEdge,i.getTexelsPerItem())),this.hasNodeAttributeData&&o&&(o.bind(et),l.u_layerAttributeTexture&&s.uniform1i(l.u_layerAttributeTexture,et),l.u_layerAttributeTextureWidth&&s.uniform1i(l.u_layerAttributeTextureWidth,o.getTextureWidth()),l.u_layerAttributeTexelsPerNode&&s.uniform1i(l.u_layerAttributeTexelsPerNode,o.getTexelsPerItem()));var u=G(this.customUniforms),c;try{for(u.s();!(c=u.n()).done;){var d=c.value;Ga(s,this.uniformLocations[d.name],d)}}catch(f){u.e(f)}finally{u.f()}r.bindAsRenderTarget(),s.disable(s.BLEND),s.disable(s.DEPTH_TEST),s.drawArrays(s.POINTS,0,a),s.bindFramebuffer(s.FRAMEBUFFER,null),s.bindVertexArray(null)}}},{key:"kill",value:function(){var n=this.gl;n.deleteProgram(this.program),n.deleteShader(this.vertexShader),n.deleteShader(this.fragmentShader),n.deleteVertexArray(this.vao)}}])})();function vn(e,t,n,r,a,i){if(e.length===1)return"".concat(r," ").concat(t,"(int pathId, ").concat(a,`) {
  return path_`).concat(e[0].name,"_").concat(n,"(").concat(i,`);
}`);var o=e.map(function(s,l){return"    case ".concat(l,": return path_").concat(s.name,"_").concat(n,"(").concat(i,");")}).join(`
`);return"".concat(r," ").concat(t,"(int pathId, ").concat(a,`) {
  switch (pathId) {
`).concat(o,`
    default: return path_`).concat(e[0].name,"_").concat(n,"(").concat(i,`);
  }
}`)}var nf=`
vec3 computeEdgeLabelBodyBounds(
  float tStart, float tEnd, float pathLength,
  float webGLThickness, float headLengthRatio, float tailLengthRatio
) {
  float visibleLength = pathLength * (tEnd - tStart);

  float headLength = headLengthRatio * webGLThickness;
  float tailLength = tailLengthRatio * webGLThickness;
  float totalNeededLength = headLength + tailLength;
  if (totalNeededLength > visibleLength && totalNeededLength > 0.0001) {
    float extremityScale = visibleLength / totalNeededLength;
    headLength *= extremityScale;
    tailLength *= extremityScale;
  }

  float bodyStartDist = tStart * pathLength + tailLength;
  float bodyEndDist = tEnd * pathLength - headLength;
  return vec3(bodyStartDist, bodyEndDist, max(bodyEndDist - bodyStartDist, 0.0));
}
`;function rf(e,t){var n=re(e),r=re(t);return`
float computeEdgeLabelAlpha(float bodyLength, float textWidthWebGL) {
  float ratio = textWidthWebGL > 0.0001 ? min(bodyLength / textWidthWebGL, 1.0) : 1.0;
  if (ratio < `.concat(n,`) return 0.0;
  if (ratio < `).concat(r,") return (ratio - ").concat(n,") / (").concat(r," - ").concat(n,`);
  return 1.0;
}
`)}var af=`
float computeEdgeLabelScreenDir(int pathId, float t, vec2 source, vec2 target, mat3 matrix) {
  vec2 tangent = queryPathTangent(pathId, t, source, target);
  return (matrix * vec3(tangent, 0.0)).x < 0.0 ? -1.0 : 1.0;
}
`,of=`
float computeEdgeLabelPerpOffset(
  float positionMode,
  float halfThickness, float marginWebGL, float halfTextHeight,
  float readingSign, float screenDir
) {
  float magnitude = halfThickness + marginWebGL + halfTextHeight;
  if (positionMode == 1.0) return magnitude * readingSign;
  if (positionMode == 2.0) return -magnitude * readingSign;
  if (positionMode == 3.0) return magnitude * screenDir;
  return 0.0;
}
`;function Ss(e){var t=e.paths,n=e.minVisibilityThreshold,r=e.fullVisibilityThreshold,a=t.some(function(s){return s.hasSharpCorners}),i=t.map(function(s){return"// --- Path: ".concat(s.name,` ---
`).concat(s.glsl,`

// Tangent/normal functions: analytical if provided, otherwise numerical
`).concat(s.analyticalTangentGlsl||ys(s.name),`

// Auto-generated fallbacks for any missing path functions
`).concat(_s(s.name,s.glsl),`

// Corner skip helpers (for paths with sharp corners like step/taxi)
`).concat(s.cornerSkipGlsl||"",`
`)}).join(`
`),o=a?`// Corner function selectors (only some paths have sharp corners)
vec2 queryGetCornerTs(int pathId, vec2 source, vec2 target) {
  switch (pathId) {
`.concat(t.map(function(s,l){return s.hasSharpCorners?"    case ".concat(l,": return path_").concat(s.name,"_getCornerTs(source, target);"):"    case ".concat(l,": return vec2(-1.0, -1.0); // No corners for ").concat(s.name)}).join(`
`),`
    default: return vec2(-1.0, -1.0);
  }
}

vec2 queryGetCornerConcavity(int pathId, vec2 source, vec2 target, float perpOffset) {
  switch (pathId) {
`).concat(t.map(function(s,l){return s.hasSharpCorners?"    case ".concat(l,": return path_").concat(s.name,"_getCornerConcavity(source, target, perpOffset);"):"    case ".concat(l,": return vec2(0.0, 0.0); // No corners for ").concat(s.name)}).join(`
`),`
    default: return vec2(0.0, 0.0);
  }
}`):"";return`
// ============================================================================
// Node data fetch (geometry texel) and per-edge clamp fetch (edge-frame texture)
// ============================================================================

`.concat(Be,`
`).concat(yt,`

// ============================================================================
// Path Functions (one block per path)
// ============================================================================

`).concat(i,`

// ============================================================================
// Path Query Selectors (dispatch by pathId)
// ============================================================================

`).concat(vn(t,"queryPathPosition","position","vec2","float t, vec2 source, vec2 target","t, source, target"),`

`).concat(vn(t,"queryPathTangent","tangent","vec2","float t, vec2 source, vec2 target","t, source, target"),`

`).concat(vn(t,"queryPathNormal","normal","vec2","float t, vec2 source, vec2 target","t, source, target"),`

`).concat(vn(t,"queryPathLength","length","float","vec2 source, vec2 target","source, target"),`

`).concat(vn(t,"queryPathTAtDistance","t_at_distance","float","float dist, vec2 source, vec2 target","dist, source, target"),`

`).concat(o,`

// ============================================================================
// Shared helpers (body bounds, alpha ramp, screen direction, perpendicular offset)
// ============================================================================

`).concat(nf,`
`).concat(rf(n,r),`
`).concat(af,`
`).concat(of,`
`)}var sf=Ge.fontSize,lf=new Map,Rs=24,Wo=(Rs+1)*2;function uf(e){var t=e.paths,n=e.fontSizeMode,r=e.headLengthRatio,a=e.tailLengthRatio,i=e.minVisibilityThreshold,o=e.fullVisibilityThreshold,s=e.keepUpright,l=n==="scaled",u=Me(),c=ke([].concat($(t),[u])),d=Tn(c),f=`#version 300 es

// Per-instance attributes
in float a_edgeIndex;
in float a_edgeAttrIndex;
in float a_baseFontSize;
in float a_totalTextWidth;
in float a_positionMode;
in float a_margin;
in float a_padding;
in vec4 a_color;
in vec4 a_id;

// Per-vertex (constant) attribute: strip vertex index
in float a_vertexIndex;

uniform mat3 u_matrix;
uniform float u_sizeRatio;
uniform float u_correctionRatio;
uniform float u_pixelRatio;
uniform float u_cameraAngle;
uniform vec2 u_resolution;
uniform sampler2D u_nodeDataTexture;
uniform int u_nodeDataTextureWidth;
uniform sampler2D u_edgeDataTexture;
uniform int u_edgeDataTextureWidth;
uniform sampler2D u_edgeFrameTexture; // Per-edge clamp (tStart, tEnd, straightenFactor, pathLength)
uniform int u_edgeFrameTextureWidth;
`.concat(l?"uniform float u_zoomSizeRatio;":"",`

`).concat(d.uniformDeclarations,`

out vec4 v_color;
out vec4 v_id;
out float v_alphaModifier;

const float ATLAS_FONT_SIZE = `).concat(re(sf),`;
const float HEAD_RATIO = `).concat(re(r),`;
const float TAIL_RATIO = `).concat(re(a),`;
const int RIBBON_SEGMENTS = `).concat(Rs,`;

// Path attribute varyings (declared as plain locals since this shader has no FS inputs for them)
`).concat(d.vertexVaryingDeclarations.replace(/out /g,""),`

// Node size variables used by some path functions (self-loops, etc.)
float v_sourceNodeSize;
float v_targetNodeSize;

// Shared preamble: shape SDFs, path functions + selectors, clamp, helpers.
`).concat(Ss({paths:t,minVisibilityThreshold:i,fullVisibilityThreshold:o}),`

void main() {
  int vIdx = int(a_vertexIndex);
  int pairIdx = vIdx / 2;
  int side = vIdx - pairIdx * 2; // 0 = left/bottom, 1 = right/top

  // --- Fetch edge data (2 texels per edge) ---
  int edgeIdx = int(a_edgeIndex);
  int texel0Idx = edgeIdx * 2;
  int texel1Idx = edgeIdx * 2 + 1;
  ivec2 e0 = ivec2(texel0Idx % u_edgeDataTextureWidth, texel0Idx / u_edgeDataTextureWidth);
  ivec2 e1 = ivec2(texel1Idx % u_edgeDataTextureWidth, texel1Idx / u_edgeDataTextureWidth);
  vec4 edgeData0 = texelFetch(u_edgeDataTexture, e0, 0);
  vec4 edgeData1 = texelFetch(u_edgeDataTexture, e1, 0);

  int srcIdx = int(edgeData0.x);
  int tgtIdx = int(edgeData0.y);
  float thickness = edgeData0.z;
  int pathId = int(edgeData1.z);

  // --- Fetch path attributes (curvature, etc.) ---
  {
    int edgeIdx = int(a_edgeAttrIndex);
`).concat(d.fetchCode,`
`).concat(d.varyingAssignments,`
  }

  // --- Fetch node data: geometry (texel 0) + rotation flags (texel 1) ---
  vec4 srcN = readNodeData(u_nodeDataTexture, u_nodeDataTextureWidth, srcIdx);
  vec4 tgtN = readNodeData(u_nodeDataTexture, u_nodeDataTextureWidth, tgtIdx);

  vec2 source = srcN.xy;
  vec2 target = tgtN.xy;
  float sourceSize = srcN.z;
  float targetSize = tgtN.z;
  v_sourceNodeSize = sourceSize;
  v_targetNodeSize = targetSize;

  // --- Pixel-to-graph conversion (fixed font mode) ---
  float matrixScaleX = length(vec2(u_matrix[0][0], u_matrix[1][0]));
  float pixelToGraph = 2.0 / (matrixScaleX * u_resolution.x);

  float webGLThickness = thickness * u_correctionRatio / u_sizeRatio;

  // --- Body bounds (shared with edge label shader) ---
  vec4 edgeClamp = readFrameTexel(u_edgeFrameTexture, u_edgeFrameTextureWidth, edgeIdx);
  vec3 bodyBounds = computeEdgeLabelBodyBounds(
    edgeClamp.x, edgeClamp.y, edgeClamp.w,
    webGLThickness, HEAD_RATIO, TAIL_RATIO
  );
  float bodyStartDist = bodyBounds.x;
  float bodyEndDist = bodyBounds.y;
  float bodyLength = bodyBounds.z;

  // --- Text dimensions ---
  float baseFontSize = a_baseFontSize;
  `).concat(l?`float fontScale = baseFontSize / ATLAS_FONT_SIZE * u_zoomSizeRatio;
  float textWidthWebGL = a_totalTextWidth * fontScale * u_correctionRatio / u_sizeRatio;
  float halfTextHeight = baseFontSize * 0.35 * u_zoomSizeRatio * u_correctionRatio / u_sizeRatio;
  float marginWebGL = a_margin * u_zoomSizeRatio * u_correctionRatio / u_sizeRatio;`:`float fontScale = baseFontSize / ATLAS_FONT_SIZE;
  float textWidthWebGL = a_totalTextWidth * fontScale * pixelToGraph;
  float halfTextHeight = baseFontSize * 0.35 * pixelToGraph;
  float marginWebGL = a_margin * pixelToGraph;`,`
  float paddingWebGL = a_padding * pixelToGraph;

  // --- Alpha modifier (shared with edge label shader) ---
  float alphaModifier = computeEdgeLabelAlpha(bodyLength, textWidthWebGL);
  if (alphaModifier <= 0.0 || textWidthWebGL <= 0.0) {
    gl_Position = vec4(2.0, 0.0, 0.0, 1.0);
    v_color = vec4(0.0);
    v_id = vec4(0.0);
    v_alphaModifier = 0.0;
    return;
  }

  // --- Perpendicular offset (shared with edge label shader) ---
  float bodyCenterDist = (bodyStartDist + bodyEndDist) * 0.5;
  float centerT = queryPathTAtDistance(pathId, bodyCenterDist, source, target);
  float screenDir = computeEdgeLabelScreenDir(pathId, centerT, source, target, u_matrix);
  float readingSign = `).concat(s?"screenDir":"1.0",`;
  float halfThickness = webGLThickness * 0.5;
  float perpOffset = computeEdgeLabelPerpOffset(
    a_positionMode, halfThickness, marginWebGL, halfTextHeight, readingSign, screenDir
  );

  // --- Ribbon span (clipped to body) ---
  float halfTextWebGL = textWidthWebGL * 0.5;
  float labelStartDist = max(bodyCenterDist - halfTextWebGL, bodyStartDist);
  float labelEndDist = min(bodyCenterDist + halfTextWebGL, bodyEndDist);

  // Sample centerline at this pair, then apply perpendicular offset.
  // The ribbon follows the centerline path (simple & robust); for curved edges
  // this closely matches the offset path the characters sit on.
  float u = float(pairIdx) / float(RIBBON_SEGMENTS);
  float arcDist = mix(labelStartDist, labelEndDist, u);
  float t = queryPathTAtDistance(pathId, arcDist, source, target);
  vec2 pos = queryPathPosition(pathId, t, source, target);
  vec2 tan = queryPathTangent(pathId, t, source, target);
  vec2 perp = vec2(-tan.y, tan.x);

  vec2 centerPos = pos + perp * perpOffset;

  float halfRibbon = halfTextHeight + paddingWebGL;
  float sideSign = side == 0 ? -1.0 : 1.0;
  vec2 ribbonPos = centerPos + perp * (sideSign * halfRibbon);

  vec3 clipPos = u_matrix * vec3(ribbonPos, 1.0);
  gl_Position = vec4(clipPos.xy, 0.0, 1.0);

  v_color = a_color;
  v_id = a_id;
  v_alphaModifier = alphaModifier;
}
`);return f}var cf=`#version 300 es
precision highp float;

in vec4 v_color;
in vec4 v_id;
in float v_alphaModifier;

out vec4 fragColor;

void main() {
#ifdef PICKING_MODE
  if (v_alphaModifier <= 0.0) discard;
  fragColor = v_id;
#else
  float alpha = v_color.a * v_alphaModifier;
  if (alpha <= 0.0) discard;
  fragColor = vec4(v_color.rgb * alpha, alpha);
#endif
}
`;function As(e,t,n,r){var a=r.shaderConfig,i=a.paths,o=a.fontSizeMode;if(i.length===0)throw new Error("createEdgeLabelBackgroundProgram: shaderConfig must declare at least one path");var s=Me(),l=ke([].concat($(i),[s])),u=kr([].concat($(i),[s]),l),c=uf(a),d=(function(f){function h(g,m,v){var p;return K(this,h),p=ae(this,h,[g,m,v]),R(p,"totalCount",0),R(p,"bufferCapacity",0),R(p,"edgeAttributeTexture",null),p.edgeAttributeTexture=new Cr(g,l),p.packedAttributeData=new Float32Array(l.floatsPerItem),p}return ie(h,f),Z(h,[{key:"getDefinition",value:function(){for(var m=WebGL2RenderingContext,v=m.FLOAT,p=m.UNSIGNED_BYTE,b=m.TRIANGLE_STRIP,y=[],_=0;_<Wo;_++)y.push([_]);var x=["u_matrix","u_sizeRatio","u_correctionRatio","u_pixelRatio","u_cameraAngle","u_resolution","u_nodeDataTexture","u_nodeDataTextureWidth","u_edgeDataTexture","u_edgeDataTextureWidth","u_edgeFrameTexture","u_edgeFrameTextureWidth"];return o==="scaled"&&x.push("u_zoomSizeRatio"),l.floatsPerItem>0&&x.push("u_edgeAttributeTexture","u_edgeAttributeTextureWidth","u_edgeAttributeTexelsPerEdge"),{VERTICES:Wo,VERTEX_SHADER_SOURCE:c,FRAGMENT_SHADER_SOURCE:cf,METHOD:b,UNIFORMS:x,ATTRIBUTES:[{name:"a_edgeIndex",size:1,type:v},{name:"a_edgeAttrIndex",size:1,type:v},{name:"a_baseFontSize",size:1,type:v},{name:"a_totalTextWidth",size:1,type:v},{name:"a_positionMode",size:1,type:v},{name:"a_margin",size:1,type:v},{name:"a_padding",size:1,type:v},{name:"a_color",size:4,type:p,normalized:!0},{name:"a_id",size:4,type:p,normalized:!0}],CONSTANT_ATTRIBUTES:[{name:"a_vertexIndex",size:1,type:v}],CONSTANT_DATA:y}}},{key:"processEdgeLabelBackground",value:function(m,v,p){var b=0;if(this.edgeAttributeTexture&&l.floatsPerItem>0){b=this.edgeAttributeTexture.allocate(v);var y=this.packedAttributeData;Lr(u,p.edgeAttributes,y,"",lf,0),this.edgeAttributeTexture.updateAllAttributes(v,y)}var _=this.floats,x=this.ints,T=m*this.STRIDE;_[T++]=p.edgeIndex,_[T++]=b,_[T++]=p.baseFontSize,_[T++]=p.totalTextWidth,_[T++]=p.positionMode,_[T++]=p.margin,_[T++]=p.padding,_[T++]=p.color,x[T++]=p.id}},{key:"setUniforms",value:function(m,v){var p=v.gl,b=v.uniformLocations;if(p.uniformMatrix3fv(b.u_matrix,!1,m.matrix),p.uniform1f(b.u_sizeRatio,m.sizeRatio),p.uniform1f(b.u_correctionRatio,m.correctionRatio),p.uniform1f(b.u_pixelRatio,m.pixelRatio),p.uniform1f(b.u_cameraAngle,m.cameraAngle),p.uniform2f(b.u_resolution,m.width,m.height),b.u_nodeDataTexture!==void 0&&p.uniform1i(b.u_nodeDataTexture,m.nodeDataTextureUnit),b.u_nodeDataTextureWidth!==void 0&&p.uniform1i(b.u_nodeDataTextureWidth,m.nodeDataTextureWidth),b.u_edgeDataTexture!==void 0&&p.uniform1i(b.u_edgeDataTexture,m.edgeDataTextureUnit),b.u_edgeDataTextureWidth!==void 0&&p.uniform1i(b.u_edgeDataTextureWidth,m.edgeDataTextureWidth),b.u_edgeFrameTexture!==void 0&&p.uniform1i(b.u_edgeFrameTexture,m.edgeFrameTextureUnit),b.u_edgeFrameTextureWidth!==void 0&&p.uniform1i(b.u_edgeFrameTextureWidth,m.edgeFrameTextureWidth),o==="scaled"&&b.u_zoomSizeRatio!==void 0){var y=this.renderer.getSetting("zoomToSizeRatioFunction");p.uniform1f(b.u_zoomSizeRatio,1/y(m.zoomRatio))}this.edgeAttributeTexture&&l.floatsPerItem>0&&b.u_edgeAttributeTexture!==void 0&&(this.edgeAttributeTexture.bind(tt),p.uniform1i(b.u_edgeAttributeTexture,tt),p.uniform1i(b.u_edgeAttributeTextureWidth,this.edgeAttributeTexture.getTextureWidth()),p.uniform1i(b.u_edgeAttributeTexelsPerEdge,this.edgeAttributeTexture.getTexelsPerItem()))}},{key:"renderProgram",value:function(m,v){this.edgeAttributeTexture&&l.floatsPerItem>0&&this.edgeAttributeTexture.upload(),de(h,"renderProgram",this,3)([m,v])}},{key:"kill",value:function(){this.edgeAttributeTexture&&(this.edgeAttributeTexture.kill(),this.edgeAttributeTexture=null),de(h,"kill",this,3)([])}},{key:"drawWebGL",value:function(m,v){var p=v.gl;this.totalCount!==0&&p.drawArraysInstanced(p.TRIANGLE_STRIP,0,this.VERTICES,this.totalCount)}},{key:"reallocate",value:function(m){this.totalCount=m,m>this.bufferCapacity&&(this.bufferCapacity=Math.max(m,Math.ceil(this.bufferCapacity*1.5)||10),de(h,"reallocate",this,3)([this.bufferCapacity]))}}])})(rt);return new d(e,t,n)}function ws(e){var t,n,r,a,i,o;return{paths:e.paths,headLengthRatio:(t=e.headLengthRatio)!==null&&t!==void 0?t:0,tailLengthRatio:(n=e.tailLengthRatio)!==null&&n!==void 0?n:0,fontSizeMode:(r=e.fontSizeMode)!==null&&r!==void 0?r:"fixed",minVisibilityThreshold:(a=e.minVisibilityThreshold)!==null&&a!==void 0?a:.7,fullVisibilityThreshold:(i=e.fullVisibilityThreshold)!==null&&i!==void 0?i:.8,keepUpright:(o=e.keepUpright)!==null&&o!==void 0?o:!0}}var df=Ge.fontSize,hf=17/64;function ff(e){var t=e.paths,n=e.hasBorder,r=n===void 0?!1:n,a=e.fontSizeMode,i=a===void 0?"fixed":a,o=e.minVisibilityThreshold,s=o===void 0?.5:o,l=e.fullVisibilityThreshold,u=l===void 0?.6:l,c=e.keepUpright,d=c===void 0?!0:c,f=i==="scaled",h=t.some(function(b){return b.hasSharpCorners}),g=Me(),m=ke([].concat($(t),[g])),v=Tn(m),p=`#version 300 es

// ============================================================================
// Attributes - Per Character (Instanced)
// ============================================================================

// Edge geometry: indices for texture lookup
// Edge data (source/target node indices, thickness, head/tail ratios) is fetched from edge data texture
// Edge path attributes (curvature, etc.) are fetched from edge attribute texture
in float a_edgeIndex;       // Index into edge data texture
in float a_edgeAttrIndex;   // Index into edge attribute texture (for curvature, etc.)
in float a_baseFontSize;    // Base font size in pixels (per-label)

// Character metrics (in glyph units = atlas font size pixels)
in vec4 a_charMetrics;      // (charTextOffset, charAdvance, totalTextWidth, positionMode)
in vec4 a_charDims;         // (charSize.x, charSize.y, charOffset.x, charOffset.y)

// Atlas texture coordinates
in vec4 a_texCoords;        // (x, y, width, height) in atlas pixels

// Label parameters
in vec2 a_labelParams;      // (margin, unused)

// Appearance
in vec4 a_color;            // Character color (RGBA, normalized)
`.concat(r?"in vec4 a_borderColor;      // Border color (RGBA, normalized)":"",`

// ============================================================================
// Attributes - Per Vertex (Constant)
// ============================================================================

in vec2 a_quadCorner;       // Quad corner: (0,0), (1,0), (0,1), (1,1)

// ============================================================================
// Uniforms
// ============================================================================

uniform mat3 u_matrix;
uniform float u_sizeRatio;
uniform float u_correctionRatio;
uniform float u_pixelRatio;
uniform float u_cameraAngle;    // Required by node shape SDFs
// u_sdfBufferPixels kept for ABI compatibility but unused in shader
uniform float u_sdfBufferPixels;
uniform vec2 u_resolution;
uniform vec2 u_atlasSize;
uniform sampler2D u_nodeDataTexture; // Shared texture with node position/size/shape data
uniform int u_nodeDataTextureWidth;  // Width of 2D node data texture for coordinate calculation
uniform sampler2D u_edgeDataTexture; // Shared texture with edge data
uniform int u_edgeDataTextureWidth;  // Width of 2D edge data texture for coordinate calculation
uniform sampler2D u_edgeFrameTexture; // Per-edge clamp (tStart, tEnd, straightenFactor, pathLength)
uniform int u_edgeFrameTextureWidth;
`).concat(f?"uniform float u_zoomSizeRatio;  // Zoom-based size ratio from zoomToSizeRatioFunction":"",`

// Edge path attribute texture uniforms (for curvature and other path attributes)
`).concat(v.uniformDeclarations,`

// ============================================================================
// Varyings
// ============================================================================

out vec2 v_texCoord;
out vec4 v_color;
`).concat(r?"out vec4 v_borderColor;":"",`
out float v_edgeFade;  // 0 = fully visible, 1 = fully faded (outside body)
out float v_alphaModifier;  // 0-1 based on label visibility ratio
out float v_fontScale;  // Ratio of rendered font size to atlas font size
`).concat(r?"out float v_positionMode;  // Position mode for conditional border (0=over needs border)":"",`

// ============================================================================
// Constants
// ============================================================================

const float bias = 255.0 / 254.0;
const float FADE_WIDTH_PIXELS = 15.0;  // Width of fade gradient in pixels
const float ATLAS_FONT_SIZE = `).concat(re(df),`;  // Base font size used in SDF atlas
const float VERTICAL_CENTER_RATIO = `).concat(re(hf),`;  // Baseline to visual center ratio

// ============================================================================
// Path Attribute Variables (set in main, used by path functions)
// ============================================================================
// Path attributes are fetched from the edge attribute texture and stored in
// variables with v_ prefix (e.g., v_curvature) for path functions to access.
`).concat(v.vertexVaryingDeclarations.replace(/out /g,""),`

// Node size variables (set in main, used by some path functions like loops).
// These mirror the v_sourceNodeSize / v_targetNodeSize varyings in generator.ts,
// but are plain floats here since the label shader is vertex-only.
float v_sourceNodeSize;
float v_targetNodeSize;

// Shared preamble: shape SDFs, path functions + selectors, clamp, helpers.
`).concat(Ss({paths:t,minVisibilityThreshold:s,fullVisibilityThreshold:u}),`

// ============================================================================
// Main
// ============================================================================

void main() {
  // -------------------------------------------------------------------------
  // Fetch edge data from edge texture (2 texels per edge)
  // -------------------------------------------------------------------------
  // Texel 0: sourceNodeIndex, targetNodeIndex, thickness, reserved
  // Texel 1: headLengthRatio, tailLengthRatio, pathId, extremityIds
  int edgeIdx = int(a_edgeIndex);
  int texel0Idx = edgeIdx * 2;
  int texel1Idx = edgeIdx * 2 + 1;
  ivec2 edgeTexCoord0 = ivec2(texel0Idx % u_edgeDataTextureWidth, texel0Idx / u_edgeDataTextureWidth);
  ivec2 edgeTexCoord1 = ivec2(texel1Idx % u_edgeDataTextureWidth, texel1Idx / u_edgeDataTextureWidth);
  vec4 edgeData0 = texelFetch(u_edgeDataTexture, edgeTexCoord0, 0);
  vec4 edgeData1 = texelFetch(u_edgeDataTexture, edgeTexCoord1, 0);

  // Unpack edge data
  int srcIdx = int(edgeData0.x);
  int tgtIdx = int(edgeData0.y);
  float thickness = edgeData0.z;
  // edgeData0.w is reserved
  float headLengthRatio = edgeData1.x;
  float tailLengthRatio = edgeData1.y;
  int pathId = int(edgeData1.z);  // Path type for multi-path support
  float baseFontSize = a_baseFontSize;

  // -------------------------------------------------------------------------
  // Fetch path attributes from edge attribute texture
  // -------------------------------------------------------------------------
  // Note: The fetch code uses 'edgeIdx' variable, so we set it to the attribute texture index
  {
    int edgeIdx = int(a_edgeAttrIndex);  // Use attribute texture index for path attributes
`).concat(v.fetchCode,`
`).concat(v.varyingAssignments,`
  }

  // -------------------------------------------------------------------------
  // Fetch node data from node texture (geometry texel + rotation-flags texel)
  // -------------------------------------------------------------------------
  vec4 srcNodeData = readNodeData(u_nodeDataTexture, u_nodeDataTextureWidth, srcIdx);
  vec4 tgtNodeData = readNodeData(u_nodeDataTexture, u_nodeDataTextureWidth, tgtIdx);

  vec2 source = srcNodeData.xy;
  vec2 target = tgtNodeData.xy;
  float sourceSize = srcNodeData.z;
  float targetSize = tgtNodeData.z;
  v_sourceNodeSize = sourceSize;
  v_targetNodeSize = targetSize;
  float charTextOffset = a_charMetrics.x;
  float charAdvance = a_charMetrics.y;
  float totalTextWidth = a_charMetrics.z;
  float positionMode = a_charMetrics.w;
  vec2 charSize = a_charDims.xy;
  vec2 charOffset = a_charDims.zw;
  float margin = a_labelParams.x;

  // -------------------------------------------------------------------------
  // Compute pixel-to-graph conversion (for fixed font size mode)
  // -------------------------------------------------------------------------
  // This converts screen pixels to graph units such that N pixels on screen
  // becomes N pixels regardless of zoom level.
  // matrixScaleX is how much the matrix scales graph units to clip space
  float matrixScaleX = length(vec2(u_matrix[0][0], u_matrix[1][0]));
  float pixelToGraph = 2.0 / (matrixScaleX * u_resolution.x);

  // -------------------------------------------------------------------------
  // Step 1: Convert thickness to WebGL units
  // -------------------------------------------------------------------------
  float webGLThickness = thickness * u_correctionRatio / u_sizeRatio;

  // -------------------------------------------------------------------------
  // Step 2: Compute body bounds (truncated at node boundaries + extremities)
  // -------------------------------------------------------------------------
  vec4 edgeClamp = readFrameTexel(u_edgeFrameTexture, u_edgeFrameTextureWidth, edgeIdx);
  vec3 bodyBounds = computeEdgeLabelBodyBounds(
    edgeClamp.x, edgeClamp.y, edgeClamp.w,
    webGLThickness, headLengthRatio, tailLengthRatio
  );
  float bodyStartDist = bodyBounds.x;
  float bodyEndDist = bodyBounds.y;
  float bodyLength = bodyBounds.z;

  // -------------------------------------------------------------------------
  // Step 3: Compute font scale and text dimensions
  // -------------------------------------------------------------------------
  // Font size modes:
  // - "fixed": Constant pixel size regardless of zoom, using pixelToGraph conversion
  // - "scaled": Scales with zoom using zoomToSizeRatioFunction
  `).concat(f?`// Scaled mode: font scales with zoom
  float fontScale = baseFontSize / ATLAS_FONT_SIZE * u_zoomSizeRatio;
  // Convert glyph-unit metrics to WebGL units (scales with zoom)
  float textWidthWebGL = totalTextWidth * fontScale * u_correctionRatio / u_sizeRatio;
  float charOffsetWebGL = charTextOffset * fontScale * u_correctionRatio / u_sizeRatio;
  float charAdvanceWebGL = charAdvance * fontScale * u_correctionRatio / u_sizeRatio;`:`// Fixed mode: font stays constant in screen pixels
  float fontScale = baseFontSize / ATLAS_FONT_SIZE;
  // Convert glyph-unit metrics to graph units using pixelToGraph (zoom-independent)
  float textWidthWebGL = totalTextWidth * fontScale * pixelToGraph;
  float charOffsetWebGL = charTextOffset * fontScale * pixelToGraph;
  float charAdvanceWebGL = charAdvance * fontScale * pixelToGraph;`,`

  // Font scale varying for fragment shader gamma scaling.
  // Ratio of rendered font size to atlas font size \u2014 used to tighten the
  // anti-aliasing band for large labels so they stay sharp.
  v_fontScale = fontScale;

  // -------------------------------------------------------------------------
  // Step 4: Alpha modifier from how much of the label fits in the body
  // -------------------------------------------------------------------------
  float alphaModifier = computeEdgeLabelAlpha(bodyLength, textWidthWebGL);

  // -------------------------------------------------------------------------
  // Step 5: Compute character center offset (truncation check moved to after curvature adjustment)
  // -------------------------------------------------------------------------
  // Body center in arc distance
  float bodyCenterDist = (bodyStartDist + bodyEndDist) * 0.5;

  float centerT = queryPathTAtDistance(pathId, bodyCenterDist, source, target);
  float screenDir = computeEdgeLabelScreenDir(pathId, centerT, source, target, u_matrix);

  // Reversed labels lay their characters out from target to source
  float readingSign = `).concat(d?"screenDir":"1.0",`;

  // Character center position relative to label center (on centerline, before curvature adjustment)
  float charCenterOffset = readingSign * (charOffsetWebGL + charAdvanceWebGL * 0.5 - textWidthWebGL * 0.5);

  // -------------------------------------------------------------------------
  // Step 6: Compute perpendicular offset based on position mode
  // -------------------------------------------------------------------------
  // Position modes: 0=over, 1=above, 2=below, 3=auto
  // Needed early for curvature-adaptive character spacing in Step 7
  float halfThickness = webGLThickness * 0.5;
  `).concat(f?`// Scaled mode: margin and text height scale with zoom (same factor as font)
  float marginWebGL = margin * u_zoomSizeRatio * u_correctionRatio / u_sizeRatio;
  float halfTextHeight = baseFontSize * 0.35 * u_zoomSizeRatio * u_correctionRatio / u_sizeRatio;`:`// Fixed mode: margin and text height stay constant in screen pixels
  float marginWebGL = margin * pixelToGraph;
  float halfTextHeight = baseFontSize * 0.35 * pixelToGraph;`,`
  float perpOffset = computeEdgeLabelPerpOffset(
    positionMode, halfThickness, marginWebGL, halfTextHeight, readingSign, screenDir
  );

  // -------------------------------------------------------------------------
  // Step 7: Position character on path using offset path traversal
  // -------------------------------------------------------------------------
  // For "over" mode (perpOffset = 0), use simple centerline placement
  // For above/below modes, walk along the offset path to find correct position
  float charT;

  if (perpOffset == 0.0) {
    // Simple case: place on centerline
    float charArcDist = bodyCenterDist + charCenterOffset;
    charT = queryPathTAtDistance(pathId, charArcDist, source, target);
  } else {
    // Offset path traversal: walk along the offset curve to find character position
    // This ensures even character spacing regardless of curvature

    `).concat(h?`// -----------------------------------------------------------------------
    // Corner skip setup for step/taxi edges with above/below labels
    // -----------------------------------------------------------------------
    // At concave corners (inner side of the bend), characters would bunch up
    // because the offset path has near-zero arc length. We detect corner
    // crossings during the offset path traversal and add skip distance.

    // Get corner t values and concavity
    vec2 cornerTs = queryGetCornerTs(pathId, source, target);
    vec2 concavity = queryGetCornerConcavity(pathId, source, target, perpOffset);

    // Skip distance in graph units, proportional to on-screen font size
    // For fixed mode: use pixelToGraph so gap stays constant regardless of zoom
    // For scaled mode: use the same conversion as text width
    `.concat(f?"float skipDistGraph = STEP_INNER_CORNER_SKIP_FACTOR * baseFontSize * u_zoomSizeRatio * u_correctionRatio / u_sizeRatio;":"float skipDistGraph = STEP_INNER_CORNER_SKIP_FACTOR * baseFontSize * pixelToGraph;",`

    // Corner t values for detecting crossings during traversal
    float corner1T = cornerTs.x;
    float corner2T = cornerTs.y;
    bool corner1IsConcave = concavity.x > 0.5;
    bool corner2IsConcave = concavity.y > 0.5;`):"",`

    // Target distance along offset path from center
    float targetOffsetDist = abs(charCenterOffset);

    // Handle center character (charCenterOffset \u2248 0) - no search needed
    if (targetOffsetDist < 0.0001) {
      charT = centerT;
    } else {
      vec2 centerPos = queryPathPosition(pathId, centerT, source, target);
      vec2 centerNormal = queryPathNormal(pathId, centerT, source, target);
      vec2 offsetCenter = centerPos + centerNormal * perpOffset;

      float searchDir = charCenterOffset > 0.0 ? 1.0 : -1.0;

      // Search bounds (t values for body start and end)
      float tBodyStart = queryPathTAtDistance(pathId, bodyStartDist, source, target);
      float tBodyEnd = queryPathTAtDistance(pathId, bodyEndDist, source, target);

      // Walk along offset path to find character position
      float accumDist = 0.0;
      vec2 prevOffsetPos = offsetCenter;
      float prevT = centerT;
      float foundT = centerT;

      // Search range depends on direction
      float tSearchEnd = searchDir > 0.0 ? tBodyEnd : tBodyStart;

      `).concat(h?`// Track which concave corners we've crossed to add skip distance
      bool crossedCorner1 = false;
      bool crossedCorner2 = false;
      float effectiveTargetDist = targetOffsetDist;`:"",`

      const int STEPS = 32;
      for (int i = 1; i <= STEPS; i++) {
        // Step along centerline t, from center toward target
        float stepT = centerT + searchDir * float(i) * abs(tSearchEnd - centerT) / float(STEPS);

        `).concat(h?`// Check for concave corner crossings and add skip distance
        // Corner 1 crossing check
        if (corner1IsConcave && !crossedCorner1) {
          bool crossingCorner1 = (searchDir > 0.0)
            ? (prevT < corner1T && stepT >= corner1T)
            : (prevT > corner1T && stepT <= corner1T);
          if (crossingCorner1) {
            crossedCorner1 = true;
            effectiveTargetDist += skipDistGraph;
          }
        }

        // Corner 2 crossing check
        if (corner2IsConcave && !crossedCorner2) {
          bool crossingCorner2 = (searchDir > 0.0)
            ? (prevT < corner2T && stepT >= corner2T)
            : (prevT > corner2T && stepT <= corner2T);
          if (crossingCorner2) {
            crossedCorner2 = true;
            effectiveTargetDist += skipDistGraph;
          }
        }`:"",`

        // Compute offset position at this t
        vec2 stepPos = queryPathPosition(pathId, stepT, source, target);
        vec2 stepNormal = queryPathNormal(pathId, stepT, source, target);
        vec2 offsetPos = stepPos + stepNormal * perpOffset;

        // Distance along offset path
        float segDist = length(offsetPos - prevOffsetPos);

        `).concat(h?`if (accumDist + segDist >= effectiveTargetDist) {
          // Interpolate within segment to find exact t
          float remaining = effectiveTargetDist - accumDist;
          float segT = remaining / max(segDist, 0.0001);
          foundT = mix(prevT, stepT, segT);
          break;
        }`:`if (accumDist + segDist >= targetOffsetDist) {
          // Interpolate within segment to find exact t
          float remaining = targetOffsetDist - accumDist;
          float segT = remaining / max(segDist, 0.0001);
          foundT = mix(prevT, stepT, segT);
          break;
        }`,`

        accumDist += segDist;
        prevOffsetPos = offsetPos;
        prevT = stepT;
        // Update foundT to last valid position in case loop exhausts without finding target
        foundT = stepT;
      }

      charT = foundT;
    }
  }

  // Get position and tangent at final character position
  vec2 pathPos = queryPathPosition(pathId, charT, source, target);
  vec2 tangent = queryPathTangent(pathId, charT, source, target);

  // Compute perpendicular direction (90 degrees from tangent)
  vec2 perpDir = vec2(-tangent.y, tangent.x);

  // Apply perpendicular offset to path position
  vec2 offsetPathPos = pathPos + perpDir * perpOffset;

  // -------------------------------------------------------------------------
  // Step 8: Build character quad
  // -------------------------------------------------------------------------
  // Character size in screen pixels
  vec2 charSizePixels = charSize * fontScale;

  // Character offset from origin to the atlas region's top-left corner.
  // bearingX/bearingY already include the SDF buffer.
  vec2 charOffsetPixels = charOffset * fontScale;

  // The character's local X offset from pathPos (which is at character center)
  float charLocalX = -charAdvance * 0.5 * fontScale;

  // Build quad position:
  // - Start at character origin (charLocalX on X axis, 0 on Y axis = baseline)
  // - Add bearing offset to get to atlas region corner
  // - Add quad corner * size to get vertex position
  vec2 quadPos;
  quadPos.x = charLocalX + charOffsetPixels.x + a_quadCorner.x * charSizePixels.x;
  // charOffset.y = -bearingY (negated), so -charOffsetPixels.y = bearingY * fontScale
  // (distance from baseline to atlas region top, positive = upward)
  // Quad corner (0,0) = bottom-left, (1,1) = top-right
  quadPos.y = -charOffsetPixels.y - charSizePixels.y * (1.0 - a_quadCorner.y);

  // Center vertically on the path by offsetting by half the visual text height
  // VERTICAL_CENTER_RATIO is the distance from baseline to visual center as a ratio of atlas font size
  float verticalCenterOffset = VERTICAL_CENTER_RATIO * ATLAS_FONT_SIZE * fontScale;
  quadPos.y -= verticalCenterOffset;

  // -------------------------------------------------------------------------
  // Step 9: Rotate quad to align with tangent
  // -------------------------------------------------------------------------
  // Rotation matrix from the reading direction
  // readingDir = (cos(angle), sin(angle)), so we can build rotation directly
  vec2 readingDir = tangent * readingSign;
  mat2 rotation = mat2(readingDir.x, readingDir.y, -readingDir.y, readingDir.x);

  // Convert pixel offset to WebGL units for rotation
  `).concat(f?"vec2 quadPosWebGL = quadPos * u_correctionRatio / u_sizeRatio; // Scaled mode":"vec2 quadPosWebGL = quadPos * pixelToGraph; // Fixed mode: use pixelToGraph for zoom-independent size",`

  // Rotate around character center on path
  vec2 rotatedOffset = rotation * quadPosWebGL;

  // Final position in graph space (using offset path position for above/below modes)
  vec2 worldPos = offsetPathPos + rotatedOffset;

  // -------------------------------------------------------------------------
  // Step 10: Transform to clip space
  // -------------------------------------------------------------------------
  vec3 clipPos = u_matrix * vec3(worldPos, 1.0);
  gl_Position = vec4(clipPos.xy, 0.0, 1.0);

  // -------------------------------------------------------------------------
  // Step 11: Texture coordinates
  // -------------------------------------------------------------------------
  // Flip Y for texture coordinates (texture Y goes down, quad Y goes up)
  vec2 texCorner = vec2(a_quadCorner.x, 1.0 - a_quadCorner.y);
  v_texCoord = (a_texCoords.xy + texCorner * a_texCoords.zw) / u_atlasSize;

  // -------------------------------------------------------------------------
  // Step 12: Pass color, border color, and alpha modifier
  // -------------------------------------------------------------------------
  v_color = a_color;
  v_color.a *= bias;
`).concat(r?`  v_borderColor = a_borderColor;
  v_borderColor.a *= bias;
  v_positionMode = positionMode;`:"",`
  v_alphaModifier = alphaModifier;

  // -------------------------------------------------------------------------
  // Step 13: Compute edge fade for soft truncation
  // -------------------------------------------------------------------------
  // Convert fade width from pixels to WebGL units
  `).concat(f?"float fadeWidthWebGL = FADE_WIDTH_PIXELS * u_correctionRatio / u_sizeRatio;":"float fadeWidthWebGL = FADE_WIDTH_PIXELS * pixelToGraph;",`

  // Compute the arc position of THIS VERTEX (not just character center)
  // The quad extends from charCenter - advance/2 to charCenter + advance/2
  // a_quadCorner.x is 0 for left edge, 1 for right edge
  float vertexLocalOffset = readingSign * (a_quadCorner.x - 0.5) * charAdvanceWebGL;
  float vertexArcOffset = charCenterOffset + vertexLocalOffset;

  // Compute distance from body edges (positive = inside body, negative = outside)
  float halfBody = bodyLength * 0.5;
  float distFromStart = vertexArcOffset + halfBody;  // Distance from body start edge
  float distFromEnd = halfBody - vertexArcOffset;    // Distance from body end edge
  float distFromEdge = min(distFromStart, distFromEnd);

  // Compute fade: 0 = fully visible (deep inside body), 1 = fully faded (at body edge)
  // Fade goes from 0 (at 2*fadeWidth inside) to 1 (at body edge)
  // This ensures text is fully transparent before reaching extremities
  v_edgeFade = 1.0 - smoothstep(0.0, fadeWidthWebGL * 2.0, distFromEdge);
}
`);return p}function gf(){var e=arguments.length>0&&arguments[0]!==void 0?arguments[0]:{},t=e.hasBorder,n=t===void 0?!1:t,r=`#version 300 es
precision highp float;

in vec2 v_texCoord;
in vec4 v_color;
`.concat(n?"in vec4 v_borderColor;":"",`
in float v_edgeFade;  // 0 = fully visible, 1 = fully faded
in float v_alphaModifier;  // 0-1 based on label visibility ratio
in float v_fontScale;  // Ratio of rendered font size to atlas font size
`).concat(n?"in float v_positionMode;  // Position mode (0=over, 1=above, 2=below, 3=auto)":"",`

uniform sampler2D u_atlas;
uniform float u_gamma;
uniform float u_sdfBuffer;
uniform float u_pixelRatio;
`).concat(n?"uniform float u_borderWidth;  // Border width in SDF units (normalized)":"",`

// Fragment output (single target - picking handled via separate pass)
out vec4 fragColor;

void main() {
  #ifdef PICKING_MODE
    // Edge labels are not pickable - discard all fragments in picking mode
    fragColor = vec4(0.0);
    discard;
  #else
  // SDF stores normalized distance: 0.5 = on edge, >0.5 = inside glyph
  float sdfValue = texture(u_atlas, v_texCoord).a;

  // Edge threshold accounting for SDF buffer padding
  float edge = 1.0 - u_sdfBuffer;

  // Scale gamma inversely with font scale so small labels get a wider AA band
  // (smoother) and large labels get a tighter band (sharper).
  float aaWidth = u_gamma / (u_pixelRatio * v_fontScale);

  // Apply edge fade for soft truncation at body boundaries
  // Also apply visibility-based alpha modifier for short edge labels
  float edgeAlpha = (1.0 - v_edgeFade) * v_alphaModifier;

`).concat(n?`  // Fill alpha: fully opaque inside the glyph
  float fillAlpha = smoothstep(edge - aaWidth, edge + aaWidth, sdfValue);

  // Only apply border for "over" position mode (v_positionMode == 0.0)
  // Labels positioned above/below/auto don't overlap the edge line and don't need borders
  if (v_positionMode < 0.5) {
    // Border rendering: compute alpha for both fill and border regions
    // Border extends from (edge - borderWidth) to edge
    float borderEdge = edge - u_borderWidth;

    // Border alpha: opaque in the border region (between borderEdge and edge)
    float borderAlpha = smoothstep(borderEdge - aaWidth, borderEdge + aaWidth, sdfValue);

    // Composite: fill on top of border
    // Border is visible where borderAlpha > 0 but fillAlpha < 1
    vec3 borderColorPremult = v_borderColor.rgb * v_borderColor.a * borderAlpha * edgeAlpha;
    vec3 fillColorPremult = v_color.rgb * v_color.a * fillAlpha * edgeAlpha;

    // Blend fill over border (fill replaces border where fill is opaque)
    float finalBorderAlpha = borderAlpha * (1.0 - fillAlpha);
    vec3 finalColor = fillColorPremult + v_borderColor.rgb * v_borderColor.a * finalBorderAlpha * edgeAlpha;
    float finalAlpha = (v_color.a * fillAlpha + v_borderColor.a * finalBorderAlpha) * edgeAlpha;

    fragColor = vec4(finalColor, finalAlpha);
  } else {
    // No border for above/below/auto positions - simple text rendering
    float finalAlpha = v_color.a * fillAlpha * edgeAlpha;
    fragColor = vec4(v_color.rgb * finalAlpha, finalAlpha);
  }`:`  // Smooth transition from transparent to opaque at glyph edge
  float alpha = smoothstep(edge - aaWidth, edge + aaWidth, sdfValue);

  // Premultiplied alpha output
  float finalAlpha = v_color.a * alpha * edgeAlpha;
  fragColor = vec4(v_color.rgb * finalAlpha, finalAlpha);`,`
  #endif
}
`);return r}function pf(e){var t=arguments.length>1&&arguments[1]!==void 0?arguments[1]:!1,n=arguments.length>2&&arguments[2]!==void 0?arguments[2]:"fixed",r=["u_matrix","u_sizeRatio","u_correctionRatio","u_pixelRatio","u_cameraAngle","u_sdfBufferPixels","u_resolution","u_atlasSize","u_atlas","u_gamma","u_sdfBuffer","u_nodeDataTexture","u_nodeDataTextureWidth","u_edgeDataTexture","u_edgeDataTextureWidth","u_edgeFrameTexture","u_edgeFrameTextureWidth","u_edgeAttributeTexture","u_edgeAttributeTextureWidth","u_edgeAttributeTexelsPerEdge"];n==="scaled"&&r.push("u_zoomSizeRatio"),t&&r.push("u_borderWidth");var a=G(e),i;try{for(a.s();!(i=a.n()).done;){var o=i.value,s=G(o.uniforms),l;try{for(s.s();!(l=s.n()).done;){var u=l.value;r.includes(u.name)||r.push(u.name)}}catch(c){s.e(c)}finally{s.f()}}}catch(c){a.e(c)}finally{a.f()}return r}function Ds(e){var t=e.hasBorder,n=t===void 0?!1:t,r=e.fontSizeMode,a=r===void 0?"fixed":r;return{vertexShader:ff(e),fragmentShader:gf({hasBorder:n}),uniforms:pf(e.paths,n,a)}}var vf=new Map;function mf(e){switch(e){case"over":return 0;case"above":return 1;case"below":return 2;case"auto":return 3;default:return 0}}function Cs(e,t,n,r){var a=r.color,i=r.margin,o=r.textBorder,s=ws(r),l=s.paths,u=s.fontSizeMode,c=s.minVisibilityThreshold,d=s.fullVisibilityThreshold,f=s.keepUpright,h=!!o,g=Ds({paths:l,hasBorder:h,fontSizeMode:u,minVisibilityThreshold:c,fullVisibilityThreshold:d,keepUpright:f}),m=(function(v){function p(b,y,_){var x;K(this,p),x=ae(this,p,[b,y,_]),R(x,"atlasTexture",null),R(x,"atlasNeedsUpdate",!1),R(x,"refreshTimeout",null),R(x,"labelGlyphCache",new Map),R(x,"edgeAttributeTexture",null);var T=Me();if(x.attributeLayout=ke([].concat($(l),[T])),x.attrDescriptors=kr([].concat($(l),[T]),x.attributeLayout),x.edgeAttributeTexture=new Cr(b,x.attributeLayout),x.packedAttributeData=new Float32Array(x.attributeLayout.floatsPerItem),x.atlasManager=new vt,x.gamma=.025,x.sdfBuffer=Ge.cutoff,x.atlasTexture=b.createTexture(),!x.atlasTexture)throw new Error("EdgeLabelProgram: failed to create atlas texture");b.bindTexture(b.TEXTURE_2D,x.atlasTexture),b.texParameteri(b.TEXTURE_2D,b.TEXTURE_WRAP_S,b.CLAMP_TO_EDGE),b.texParameteri(b.TEXTURE_2D,b.TEXTURE_WRAP_T,b.CLAMP_TO_EDGE),b.texParameteri(b.TEXTURE_2D,b.TEXTURE_MIN_FILTER,b.LINEAR),b.texParameteri(b.TEXTURE_2D,b.TEXTURE_MAG_FILTER,b.LINEAR),b.bindTexture(b.TEXTURE_2D,null),x.atlasManager.on(vt.ATLAS_UPDATED_EVENT,function(){x.atlasNeedsUpdate=!0,x.refreshTimeout!==null&&clearTimeout(x.refreshTimeout),x.refreshTimeout=setTimeout(function(){x.refreshTimeout=null,x.renderer.refresh()},0)});var E={family:"sans-serif",weight:"normal",style:"normal"};return x.defaultFontKey=x.atlasManager.registerFont(E),x}return ie(p,v),Z(p,[{key:"getDefinition",value:function(){var y=WebGL2RenderingContext,_=y.FLOAT,x=y.UNSIGNED_BYTE,T=y.TRIANGLE_STRIP,E=new Set,S=[],w=G(l),A;try{for(w.s();!(A=w.n()).done;){var C=A.value,D=G(C.attributes),F;try{for(D.s();!(F=D.n()).done;){var k=F.value,L=k.name.startsWith("a_")?k.name:"a_".concat(k.name);E.has(L)||(E.add(L),S.push({name:L,size:k.size,type:k.type}))}}catch(z){D.e(z)}finally{D.f()}}}catch(z){w.e(z)}finally{w.f()}return{VERTICES:4,VERTEX_SHADER_SOURCE:g.vertexShader,FRAGMENT_SHADER_SOURCE:g.fragmentShader,METHOD:T,UNIFORMS:g.uniforms,ATTRIBUTES:[{name:"a_edgeIndex",size:1,type:_},{name:"a_edgeAttrIndex",size:1,type:_},{name:"a_baseFontSize",size:1,type:_},{name:"a_charMetrics",size:4,type:_},{name:"a_charDims",size:4,type:_},{name:"a_texCoords",size:4,type:_},{name:"a_labelParams",size:2,type:_},{name:"a_color",size:4,type:x,normalized:!0}].concat($(h?[{name:"a_borderColor",size:4,type:x,normalized:!0}]:[]),$(S.filter(function(z){return!["a_curvature","curvature"].includes(z.name)}))),CONSTANT_ATTRIBUTES:[{name:"a_quadCorner",size:2,type:_}],CONSTANT_DATA:[[0,0],[1,0],[0,1],[1,1]]}}},{key:"prepareLabelGlyphs",value:function(y,_){if(_.hidden||!_.text){this.labelGlyphCache.delete(y);return}var x=_.text,T=_.fontKey||this.defaultFontKey;this.atlasManager.ensureGlyphs(x,T);var E=[],S=[],w=0,A=G(x),C;try{for(A.s();!(C=A.n()).done;){var D=C.value,F=D.codePointAt(0);if(F===void 0){E.push(void 0),S.push(w);continue}var k=this.atlasManager.getGlyph(F,T);E.push(k),S.push(w),k&&(w+=k.advance)}}catch(L){A.e(L)}finally{A.f()}this.labelGlyphCache.set(y,{glyphs:E,xOffsets:S,totalWidth:w})}},{key:"processCharacter",value:function(y,_,x,T){var E,S,w,A=this.floats,C=this.STRIDE,D=y*C,F=this.labelGlyphCache.get(_.parentKey);if(!F||!F.glyphs[T]){for(var k=0;k<C;k++)A[D+k]=0;return}var L=F.glyphs[T],z=F.xOffsets[T],N=p.labelColor,P=N&&J(N)==="object"&&"color"in N&&N.color?N.color:_.color,I=xe(P),M=D;A[M++]=_.edgeIndex,A[M++]=(E=(S=this.edgeAttributeTexture)===null||S===void 0?void 0:S.getIndex(_.parentKey))!==null&&E!==void 0?E:0,A[M++]=_.size,A[M++]=z,A[M++]=L.advance,A[M++]=F.totalWidth,A[M++]=mf(_.position),A[M++]=L.atlasWidth,A[M++]=L.atlasHeight,A[M++]=L.bearingX,A[M++]=-L.bearingY,A[M++]=L.atlasX,A[M++]=L.atlasY,A[M++]=L.atlasWidth,A[M++]=L.atlasHeight;var j=(w=p.labelMargin)!==null&&w!==void 0?w:_.margin;if(A[M++]=j,A[M++]=0,A[M++]=I,h&&o){var U;typeof o.color=="string"?U=o.color:U=o.color.color||"#ffffff",A[M++]=xe(U)}var Y=new Set,q=G(l),X;try{for(q.s();!(X=q.n()).done;){var Le=X.value,ye=G(Le.attributes),Ve;try{for(ye.s();!(Ve=ye.n()).done;){var wt=Ve.value,Qt=wt.name.startsWith("a_")?wt.name.slice(2):wt.name;if(Qt!=="curvature"&&!Y.has(Qt)){Y.add(Qt);for(var en=0;en<wt.size;en++)A[M++]=0}}}catch(tn){ye.e(tn)}finally{ye.f()}}}catch(tn){q.e(tn)}finally{q.f()}}},{key:"processEdgeLabel",value:function(y,_,x){if(this.prepareLabelGlyphs(y,x),this.edgeAttributeTexture&&!x.hidden&&x.text){this.edgeAttributeTexture.allocate(y);var T=this.packedAttributeData;Lr(this.attrDescriptors,x.edgeAttributes,T,"",vf,0),this.edgeAttributeTexture.updateAllAttributes(y,T)}return de(p,"processLabel",this,3)([y,_,x])}},{key:"updateAtlasTexture",value:function(){if(this.atlasNeedsUpdate){var y=this.normalProgram.gl,_=this.atlasManager.getTextures();if(_.length!==0){var x=_[0];y.bindTexture(y.TEXTURE_2D,this.atlasTexture),y.texImage2D(y.TEXTURE_2D,0,y.RGBA,x.width,x.height,0,y.RGBA,y.UNSIGNED_BYTE,x.data),y.bindTexture(y.TEXTURE_2D,null),this.atlasNeedsUpdate=!1}}}},{key:"setUniforms",value:function(y,_){var x=_.gl,T=_.uniformLocations,E=this.atlasManager.getTextures(),S=E.length>0?[E[0].width,E[0].height]:[1,1];if(x.uniformMatrix3fv(T.u_matrix,!1,y.matrix),x.uniform1f(T.u_sizeRatio,y.sizeRatio),x.uniform1f(T.u_correctionRatio,y.correctionRatio),x.uniform1f(T.u_pixelRatio,y.pixelRatio),x.uniform1f(T.u_cameraAngle,y.cameraAngle),x.uniform1f(T.u_sdfBufferPixels,Ge.buffer),x.uniform2f(T.u_resolution,y.width,y.height),x.uniform2f(T.u_atlasSize,S[0],S[1]),x.uniform1f(T.u_gamma,this.gamma),x.uniform1f(T.u_sdfBuffer,this.sdfBuffer),x.activeTexture(x.TEXTURE0),x.bindTexture(x.TEXTURE_2D,this.atlasTexture),x.uniform1i(T.u_atlas,0),T.u_nodeDataTexture!==void 0&&x.uniform1i(T.u_nodeDataTexture,y.nodeDataTextureUnit),T.u_nodeDataTextureWidth!==void 0&&x.uniform1i(T.u_nodeDataTextureWidth,y.nodeDataTextureWidth),T.u_edgeDataTexture!==void 0&&x.uniform1i(T.u_edgeDataTexture,y.edgeDataTextureUnit),T.u_edgeDataTextureWidth!==void 0&&x.uniform1i(T.u_edgeDataTextureWidth,y.edgeDataTextureWidth),T.u_edgeFrameTexture!==void 0&&x.uniform1i(T.u_edgeFrameTexture,y.edgeFrameTextureUnit),T.u_edgeFrameTextureWidth!==void 0&&x.uniform1i(T.u_edgeFrameTextureWidth,y.edgeFrameTextureWidth),h&&o&&T.u_borderWidth!==void 0){var w=o.width/Ge.buffer*this.sdfBuffer;x.uniform1f(T.u_borderWidth,w)}if(this.edgeAttributeTexture&&T.u_edgeAttributeTexture!==void 0&&(this.edgeAttributeTexture.bind(tt),x.uniform1i(T.u_edgeAttributeTexture,tt),x.uniform1i(T.u_edgeAttributeTextureWidth,this.edgeAttributeTexture.getTextureWidth()),x.uniform1i(T.u_edgeAttributeTexelsPerEdge,this.edgeAttributeTexture.getTexelsPerItem())),u==="scaled"&&T.u_zoomSizeRatio!==void 0){var A=this.renderer.getSetting("zoomToSizeRatioFunction"),C=1/A(y.zoomRatio);x.uniform1f(T.u_zoomSizeRatio,C)}}},{key:"renderProgram",value:function(y,_){this.updateAtlasTexture(),this.atlasManager.hasPendingGlyphs()&&(this.atlasManager.flush(),this.updateAtlasTexture()),this.edgeAttributeTexture&&this.edgeAttributeTexture.upload(),de(p,"renderProgram",this,3)([y,_])}},{key:"registerFont",value:function(y){var _=arguments.length>1&&arguments[1]!==void 0?arguments[1]:"normal",x=arguments.length>2&&arguments[2]!==void 0?arguments[2]:"normal";return this.atlasManager.registerFont({family:y,weight:_,style:x})}},{key:"getAtlasManager",value:function(){return this.atlasManager}},{key:"ensureGlyphsReady",value:function(y,_){var x=_||this.defaultFontKey,T=G(y),E;try{for(T.s();!(E=T.n()).done;){var S=E.value;this.atlasManager.ensureGlyphs(S,x)}}catch(w){T.e(w)}finally{T.f()}this.atlasManager.flush()}},{key:"measureLabelAtlasWidth",value:function(y,_){var x=_||this.defaultFontKey;this.atlasManager.ensureGlyphs(y,x),this.atlasManager.hasPendingGlyphs()&&this.atlasManager.flush();var T=0,E=G(y),S;try{for(E.s();!(S=E.n()).done;){var w=S.value,A=w.codePointAt(0);if(A!==void 0){var C=this.atlasManager.getGlyph(A,x);C&&(T+=C.advance)}}}catch(D){E.e(D)}finally{E.f()}return T}},{key:"measureLabel",value:function(y,_,x){var T=this.measureLabelAtlasWidth(y,x),E=_/Ge.fontSize;return{width:T*E,height:_,textHeight:_}}},{key:"kill",value:function(){var y=this.normalProgram.gl;this.refreshTimeout!==null&&(clearTimeout(this.refreshTimeout),this.refreshTimeout=null),this.atlasTexture&&(y.deleteTexture(this.atlasTexture),this.atlasTexture=null),this.edgeAttributeTexture&&(this.edgeAttributeTexture.kill(),this.edgeAttributeTexture=null),this.atlasManager.destroy(),this.labelGlyphCache.clear(),de(p,"kill",this,3)([])}}])})(Ba);return R(m,"labelColor",a),R(m,"labelMargin",i),new m(e,t,n)}function bf(){var e=`
// No extremity - always returns positive (outside)
float extremity_none(vec2 uv, float lengthRatio, float widthRatio) {
  return 1.0;
}
`;return{name:"none",glsl:e,length:0,widthFactor:1,margin:0,uniforms:[],attributes:[]}}function ja(e,t,n,r,a,i,o){var s,l,u=xs(r),c=u.paths,d=u.layers,f=u.defaultHead,h=u.defaultTail,g=[bf()].concat($(u.extremities)),m={},v={};c.forEach(function(D,F){return m[D.name]=F}),g.forEach(function(D,F){return v[D.name]=F});var p=(s=v[f])!==null&&s!==void 0?s:0,b=(l=v[h])!==null&&l!==void 0?l:0,y=null,_=ke([].concat($(c),$(d))),x=(function(D){function F(k,L,z){var N;K(this,F),y||(y=Pa({paths:c,extremities:g,layers:d,antialias:a})),N=ae(this,F,[k,L,z]),R(N,"layerLifecycles",new Map),R(N,"needsShaderRegeneration",!1),R(N,"edgeAttributeTexture",null),R(N,"layout",_),R(N,"attrDescriptors",[]),R(N,"lifecycleIndexOffset",c.length),N._pickingBuffer=L,N.edgeAttributeTexture=new Cr(k,N.layout),N.packedAttributeData=new Float32Array(N.layout.floatsPerItem),d.forEach(function(I,M){if(I.lifecycle){var j={gl:k,renderer:{refresh:function(){return z.refresh()}},getUniformLocation:function(Y){return k.getUniformLocation(N.normalProgram.program,Y)},requestShaderRegeneration:function(){N.needsShaderRegeneration=!0},requestRefresh:function(){z.refresh()}};N.layerLifecycles.set(M,I.lifecycle(j))}}),N.layerLifecycles.forEach(function(I){var M;return(M=I.init)===null||M===void 0?void 0:M.call(I)});var P=new Map;return N.layerLifecycles.forEach(function(I,M){I.getAttributeData&&P.set(c.length+M,I)}),N.attrDescriptors=kr([].concat($(c),$(d)),N.layout,P),N}return ie(F,D),Z(F,[{key:"getAttributeTexture",value:function(){return this.edgeAttributeTexture}},{key:"resolveEdgeIds",value:function(L,z,N){var P=L,I=0,M=p,j=b,U=z?P.selfLoopPath:N&&P.parallelPath?P.parallelPath:P.path;U&&m[U]!==void 0&&(I=m[U]),P.head&&P.head!=="none"&&v[P.head]!==void 0&&(M=v[P.head]),P.tail&&P.tail!=="none"&&v[P.tail]!==void 0&&(j=v[P.tail]);var Y=g[M],q=g[j];return{pathId:I,headId:M,tailId:j,headLengthRatio:ze(Y.length)?0:Y.length,tailLengthRatio:ze(q.length)?0:q.length}}},{key:"getDefinition",value:function(){var L=WebGL2RenderingContext,z=L.TRIANGLE_STRIP,N=z,P=y;return{VERTICES:P.verticesPerEdge,VERTEX_SHADER_SOURCE:P.vertexShader,FRAGMENT_SHADER_SOURCE:P.fragmentShader,METHOD:N,UNIFORMS:P.uniforms,ATTRIBUTES:P.attributes,CONSTANT_ATTRIBUTES:P.constantAttributes,CONSTANT_DATA:P.constantData}}},{key:"maybeRegenerateShaders",value:function(){var L=this;if(this.needsShaderRegeneration){this.needsShaderRegeneration=!1;var z=d.map(function(Y,q){var X=L.layerLifecycles.get(q);return X!=null&&X.regenerate?X.regenerate():Y});y=Pa({paths:c,extremities:g,layers:z,antialias:this.renderer.getSetting("antialiasEdges")});var N=this.normalProgram.gl,P=this.normalProgram,I=P.program,M=P.buffer,j=P.vertexShader,U=P.fragmentShader;N.deleteProgram(I),N.deleteBuffer(M),N.deleteShader(j),N.deleteShader(U),this.normalProgram=this.getProgramInfo("normal",N,y.vertexShader,y.fragmentShader,this._pickingBuffer)}}},{key:"process",value:function(L,z,N,P,I,M){var j=z*this.STRIDE;if(I.visibility==="hidden"||N.visibility==="hidden"||P.visibility==="hidden"){for(var U=j+this.STRIDE;j<U;j++)this.floats[j]=0;this.floats[z*this.STRIDE]=ns;return}this.processVisibleItem(ft(L),j,N,P,I,M)}},{key:"processVisibleItem",value:function(L,z,N,P,I,M){var j,U=this.floats,Y=this.ints;U[z++]=M,U[z++]=xe(I.color),Y[z++]=L,U[z++]=(j=I.opacity)!==null&&j!==void 0?j:1;var q=this.packedAttributeData;Lr(this.attrDescriptors,I,q,I.color,this.layerLifecycles,this.lifecycleIndexOffset),this.edgeAttributeTexture.updateAllAttributesAtRow(M,q)}},{key:"setUniforms",value:function(L,z){var N=this,P=z.gl,I=z.uniformLocations;I.u_matrix&&P.uniformMatrix3fv(I.u_matrix,!1,L.matrix),I.u_sizeRatio&&P.uniform1f(I.u_sizeRatio,L.sizeRatio),I.u_correctionRatio&&P.uniform1f(I.u_correctionRatio,L.correctionRatio),I.u_zoomRatio&&P.uniform1f(I.u_zoomRatio,L.zoomRatio),I.u_pixelRatio&&P.uniform1f(I.u_pixelRatio,L.pixelRatio),I.u_cameraAngle&&P.uniform1f(I.u_cameraAngle,L.cameraAngle),I.u_feather&&P.uniform1f(I.u_feather,L.antiAliasingFeather),I.u_minEdgeThickness&&P.uniform1f(I.u_minEdgeThickness,L.minEdgeThickness),I.u_pickingPadding&&P.uniform1f(I.u_pickingPadding,L.edgePickingPadding),I.u_nodeDataTexture&&P.uniform1i(I.u_nodeDataTexture,L.nodeDataTextureUnit),I.u_nodeDataTextureWidth&&P.uniform1i(I.u_nodeDataTextureWidth,L.nodeDataTextureWidth),I.u_edgeDataTexture&&P.uniform1i(I.u_edgeDataTexture,L.edgeDataTextureUnit),I.u_edgeDataTextureWidth&&P.uniform1i(I.u_edgeDataTextureWidth,L.edgeDataTextureWidth),I.u_edgeFrameTexture&&P.uniform1i(I.u_edgeFrameTexture,L.edgeFrameTextureUnit),I.u_edgeFrameTextureWidth&&P.uniform1i(I.u_edgeFrameTextureWidth,L.edgeFrameTextureWidth),this.edgeAttributeTexture&&this.layout.floatsPerItem>0&&(this.edgeAttributeTexture.bind(tt),I.u_edgeAttributeTexture&&P.uniform1i(I.u_edgeAttributeTexture,tt),I.u_edgeAttributeTextureWidth&&P.uniform1i(I.u_edgeAttributeTextureWidth,this.edgeAttributeTexture.getTextureWidth()),I.u_edgeAttributeTexelsPerEdge&&P.uniform1i(I.u_edgeAttributeTexelsPerEdge,this.edgeAttributeTexture.getTexelsPerItem()));var M=new Set;c.forEach(function(j){j.uniforms.forEach(function(U){M.has(U.name)||(M.add(U.name),N.setTypedUniform(U,z))})}),g.forEach(function(j){j.uniforms.forEach(function(U){M.has(U.name)||(M.add(U.name),N.setTypedUniform(U,z))})}),d.forEach(function(j){j.uniforms.forEach(function(U){M.has(U.name)||(M.add(U.name),N.setTypedUniform(U,z))})})}},{key:"render",value:function(L,z,N){this.maybeRegenerateShaders(),de(F,"render",this,3)([L,z,N])}},{key:"renderProgram",value:function(L,z){this.layerLifecycles.forEach(function(N){var P;return(P=N.beforeRender)===null||P===void 0?void 0:P.call(N)}),de(F,"renderProgram",this,3)([L,z])}},{key:"uploadAttributeTexture",value:function(){this.edgeAttributeTexture&&this.layout.floatsPerItem>0&&this.edgeAttributeTexture.upload()}},{key:"kill",value:function(){this.layerLifecycles.forEach(function(L){var z;return(z=L.kill)===null||z===void 0?void 0:z.call(L)}),this.edgeAttributeTexture&&(this.edgeAttributeTexture.kill(),this.edgeAttributeTexture=null),de(F,"kill",this,3)([])}}])})(rt),T=g[p],E=g[b],S=B({paths:c,headLengthRatio:ze(T.length)?0:T.length,tailLengthRatio:ze(E.length)?0:E.length},r.label),w=ws(S),A=function(){return{labelProgram:Cs(e,null,n,S),labelBackgroundProgram:As(e,t,n,{shaderConfig:w})}},C=new Es(e,{paths:c,extremities:g,layers:d,nodeShapes:i,nodeLayers:o});return{edgeProgram:new x(e,null,n),createLabelPrograms:A,framePass:C}}function yf(e){return J(e)==="object"&&"uniforms"in e&&Array.isArray(e.uniforms)}function _f(e){return J(e)==="object"&&"uniforms"in e&&"attributes"in e&&"glsl"in e}function xf(e){return J(e)==="object"&&"uniforms"in e&&"attributes"in e&&"segments"in e}function Tf(e){return J(e)==="object"&&"uniforms"in e&&"attributes"in e&&"length"in e}function Ef(e){return J(e)==="object"&&"uniforms"in e&&"attributes"in e&&"glsl"in e}function Sf(e){return J(e)==="object"&&"glsl"in e&&"name"in e&&!("uniforms"in e)}function Rf(e){return J(e)==="object"&&"glsl"in e&&"name"in e&&!("uniforms"in e)}function Af(e){return J(e)==="object"&&"glsl"in e&&"name"in e&&!("uniforms"in e)}function wf(e){if(yf(e))return e;if(Sf(e))return{name:e.name,glsl:e.glsl,inradiusFactor:e.inradiusFactor,uniforms:[]};throw new Error("Invalid node shape specification: ".concat(JSON.stringify(e)))}function Df(e){if(_f(e))return e;if(fo(e))return{name:e.name,glsl:e.glsl,uniforms:[],attributes:[]};throw new Error("Invalid node layer specification: ".concat(JSON.stringify(e)))}function Cf(e){if(xf(e))return e;if(Rf(e))return{name:e.name,glsl:e.glsl,segments:e.segments,uniforms:[],attributes:[]};throw new Error("Invalid edge path specification: ".concat(JSON.stringify(e)))}function kf(e){if(Ef(e))return e;if(go(e))return{name:e.name,glsl:e.glsl,uniforms:[],attributes:[]};throw new Error("Invalid edge layer specification: ".concat(JSON.stringify(e)))}function Lf(e){if(Tf(e))return e;if(Af(e))return{name:e.name,glsl:e.glsl,length:e.length,widthFactor:e.widthFactor,margin:0,uniforms:[],attributes:[]};throw new Error("Invalid edge extremity specification: ".concat(JSON.stringify(e)))}function ks(e){var t,n,r=(t=e?.shapes)!==null&&t!==void 0?t:fn.shapes,a=(n=e?.layers)!==null&&n!==void 0?n:fn.layers,i=r.map(wf),o=a.map(Df);if(i.length===0)throw new Error("At least one node shape must be specified.");if(o.length===0)throw new Error("At least one node layer must be specified.");return{shapes:i,layers:o}}function Ff(e){var t,n,r,a=(t=e?.paths)!==null&&t!==void 0?t:zt.paths,i=(n=e?.extremities)!==null&&n!==void 0?n:zt.extremities,o=(r=e?.layers)!==null&&r!==void 0?r:zt.layers,s=a.map(Cf),l=i.map(Lf).filter(function(c){return c!==null}),u=o.map(kf);if(s.length===0)throw new Error("At least one edge path must be specified.");if(u.length===0)throw new Error("At least one edge layer must be specified.");return{paths:s,extremities:l,layers:u}}function Ls(e,t){var n={},r=G(e),a;try{for(r.s();!(a=r.n()).done;){var i=a.value;i.variables&&Object.assign(n,i.variables)}}catch(o){r.e(o)}finally{r.f()}return Object.assign(n,t||{}),n}function Fs(e,t,n,r,a){var i=ks(r),o=i.shapes,s=i.layers,l=Ls(o,r?.variables),u=Oa(e,t,n,{shapes:o,layers:s,label:r?.label,backdrop:r?.backdrop,labelAttachments:r?.labelAttachments},a);return B(B({},u),{},{variables:l})}function Ps(e,t,n,r,a,i){var o=Ff(r),s=o.paths,l=o.extremities,u=o.layers,c=ks(i),d=c.shapes,f=c.layers,h=Ls(s,r?.variables),g=ja(e,t,n,{paths:s,extremities:l,layers:u,defaultHead:r?.defaultHead,defaultTail:r?.defaultTail,label:r?.label},a,d,f);return B(B({},g),{},{variables:h,paths:s})}var Pf=2;function Is(e,t,n){var r=arguments.length>3&&arguments[3]!==void 0?arguments[3]:Pf,a=t.canvas,i=a.width,o=a.height,s=n.x,l=n.y,u=n.rowHeight,c=n.maxRowWidth,d={},f=[],h=G(e),g;try{for(h.s();!(g=h.n()).done;){var m=g.value,v=m.width+r,p=m.height+r;if(v>i||p>o||s+v>i&&l+u+p>o){f.push(m);continue}s+v>i&&(c=Math.max(c,s),s=0,l+=u,u=p),m.draw(t,s,l),d[m.key]={x:s,y:l,width:m.width,height:m.height},s+=v,u=Math.max(u,p)}}catch(b){h.e(b)}finally{h.f()}return c=Math.max(c,s),{atlas:d,cursor:{x:s,y:l,rowHeight:u,maxRowWidth:c},remaining:f}}function Te(e,t,n,r){var a=Object.defineProperty;try{a({},"",{})}catch{a=0}Te=function(i,o,s,l){function u(c,d){Te(i,c,function(f){return this._invoke(c,d,f)})}o?a?a(i,o,{value:s,enumerable:!l,configurable:!l,writable:!l}):i[o]=s:(u("next",0),u("throw",1),u("return",2))},Te(e,t,n,r)}function Ee(){var e,t,n=typeof Symbol=="function"?Symbol:{},r=n.iterator||"@@iterator",a=n.toStringTag||"@@toStringTag";function i(h,g,m,v){var p=g&&g.prototype instanceof s?g:s,b=Object.create(p.prototype);return Te(b,"_invoke",(function(y,_,x){var T,E,S,w=0,A=x||[],C=!1,D={p:0,n:0,v:e,a:F,f:F.bind(e,4),d:function(k,L){return T=k,E=0,S=e,D.n=L,o}};function F(k,L){for(E=k,S=L,t=0;!C&&w&&!z&&t<A.length;t++){var z,N=A[t],P=D.p,I=N[2];k>3?(z=I===L)&&(S=N[(E=N[4])?5:(E=3,3)],N[4]=N[5]=e):N[0]<=P&&((z=k<2&&P<N[1])?(E=0,D.v=L,D.n=N[1]):P<I&&(z=k<3||N[0]>L||L>I)&&(N[4]=k,N[5]=L,D.n=I,E=0))}if(z||k>1)return o;throw C=!0,L}return function(k,L,z){if(w>1)throw TypeError("Generator is already running");for(C&&L===1&&F(L,z),E=L,S=z;(t=E<2?e:S)||!C;){T||(E?E<3?(E>1&&(D.n=-1),F(E,S)):D.n=S:D.v=S);try{if(w=2,T){if(E||(k="next"),t=T[k]){if(!(t=t.call(T,S)))throw TypeError("iterator result is not an object");if(!t.done)return t;S=t.value,E<2&&(E=0)}else E===1&&(t=T.return)&&t.call(T),E<2&&(S=TypeError("The iterator does not provide a '"+k+"' method"),E=1);T=e}else if((t=(C=D.n<0)?S:y.call(_,D))!==o)break}catch(N){T=e,E=1,S=N}finally{w=1}}return{value:t,done:C}}})(h,m,v),!0),b}var o={};function s(){}function l(){}function u(){}t=Object.getPrototypeOf;var c=[][r]?t(t([][r]())):(Te(t={},r,function(){return this}),t),d=u.prototype=s.prototype=Object.create(c);function f(h){return Object.setPrototypeOf?Object.setPrototypeOf(h,u):(h.__proto__=u,Te(h,a,"GeneratorFunction")),h.prototype=Object.create(d),h}return l.prototype=u,Te(d,"constructor",u),Te(u,"constructor",l),l.displayName="GeneratorFunction",Te(u,a,"GeneratorFunction"),Te(d),Te(d,a,"Generator"),Te(d,r,function(){return this}),Te(d,"toString",function(){return"[object Generator]"}),(Ee=function(){return{w:i,m:f}})()}function Ns(e,t,n,r,a,i,o){try{var s=e[i](o),l=s.value}catch(u){return void n(u)}s.done?t(l):Promise.resolve(l).then(r,a)}function Vt(e){return function(){var t=this,n=arguments;return new Promise(function(r,a){var i=e.apply(t,n);function o(l){Ns(i,r,a,o,s,"next",l)}function s(l){Ns(i,r,a,o,s,"throw",l)}o(void 0)})}}var Pr=new Map,zs=!1;function If(e){return new Promise(function(t,n){var r=new FileReader;r.onload=function(){return t(r.result)},r.onerror=n,r.readAsDataURL(e)})}function Nf(e){if(!Pr.has(e)){var t=fetch(e).then(function(n){return n.blob()}).then(If).catch(function(){return Pr.delete(e),null});Pr.set(e,t)}return Pr.get(e)}function zf(e){return $a.apply(this,arguments)}function $a(){return $a=Vt(Ee().m(function e(t){var n,r,a;return Ee().w(function(i){for(;;)switch(i.n){case 0:return n=document.createElement("div"),n.innerHTML=t,r=Array.from(n.querySelectorAll("img[src]")),a=r.filter(function(o){return!o.getAttribute("src").startsWith("data:")}),i.n=1,Promise.all(a.map((function(){var o=Vt(Ee().m(function s(l){var u;return Ee().w(function(c){for(;;)switch(c.n){case 0:return c.n=1,Nf(l.getAttribute("src"));case 1:u=c.v,u&&l.setAttribute("src",u);case 2:return c.a(2)}},s)}));return function(s){return o.apply(this,arguments)}})()));case 1:return i.a(2,n.innerHTML)}},e)})),$a.apply(this,arguments)}function Ms(e){return Va.apply(this,arguments)}function Va(){return Va=Vt(Ee().m(function e(t){var n,r,a,i,o,s,l,u,c,d,f,h,g,m=arguments;return Ee().w(function(v){for(;;)switch(v.n){case 0:if(n=m.length>1&&m[1]!==void 0?m[1]:1,r=t instanceof SVGElement?new XMLSerializer().serializeToString(t):t,a=new DOMParser,i=a.parseFromString(r,"image/svg+xml"),o=i.querySelector("svg"),o){v.n=1;break}return v.a(2,null);case 1:if(s=parseFloat(o.getAttribute("width")||""),l=parseFloat(o.getAttribute("height")||""),(!(s>0)||!(l>0))&&(u=o.getAttribute("viewBox"),u&&(c=u.trim().split(/[\s,]+/),s=parseFloat(c[2]),l=parseFloat(c[3]))),!(!(s>0)||!(l>0))){v.n=2;break}return console.warn("Sigma: SVG label attachment has no parseable dimensions \u2014 skipped."),v.a(2,null);case 2:return d=Math.ceil(s*n),f=Math.ceil(l*n),h=new Blob([r],{type:"image/svg+xml"}),g=URL.createObjectURL(h),v.a(2,new Promise(function(p){var b=new Image;b.onload=function(){var y=document.createElement("canvas");y.width=d,y.height=f,y.getContext("2d").drawImage(b,0,0,d,f),URL.revokeObjectURL(g);try{y.getContext("2d").getImageData(0,0,1,1)}catch(_){if(_ instanceof DOMException&&_.name==="SecurityError"){zs||(zs=!0,console.warn('Sigma: A label attachment was skipped because the rendered canvas is tainted. SVG with <foreignObject> (used by the "html" attachment type) is blocked in Chromium and Safari. Use type: "canvas" with Canvas 2D rendering instead.')),p(null);return}throw _}p(y)},b.onerror=function(){URL.revokeObjectURL(g),p(null)},b.src=g}))}},e)})),Va.apply(this,arguments)}function Mf(e,t){var n=document.createElement("div");n.style.cssText="position:fixed;left:-99999px;top:0;visibility:hidden;width:max-content;height:max-content",n.innerHTML=(t?"<style>".concat(t,"</style>"):"")+e,document.body.appendChild(n);var r=n.getBoundingClientRect(),a=r.width,i=r.height;return document.body.removeChild(n),{width:Math.ceil(a),height:Math.ceil(i)}}function Gf(e,t,n,r){return qa.apply(this,arguments)}function qa(){return qa=Vt(Ee().m(function e(t,n,r,a){var i,o,s,l,u,c,d,f,h=arguments;return Ee().w(function(g){for(;;)switch(g.n){case 0:return i=h.length>4&&h[4]!==void 0?h[4]:1,o=t instanceof HTMLElement?t.outerHTML:t,g.n=1,zf(o);case 1:return s=g.v,(r==null||a==null)&&(l=Mf(s,n),r=r??l.width,a=a??l.height),u=Math.ceil(r*i),c=Math.ceil(a*i),d=n?"<style>".concat(n,"</style>"):"",f='<svg xmlns="http://www.w3.org/2000/svg" '+'width="'.concat(u,'" height="').concat(c,'" viewBox="0 0 ').concat(r," ").concat(a,'">')+'<foreignObject width="'.concat(r,'" height="').concat(a,'">')+'<body xmlns="http://www.w3.org/1999/xhtml" style="margin:0;padding:0">'.concat(d).concat(s,"</body>")+"</foreignObject></svg>",g.a(2,Ms(f,1))}},e)})),qa.apply(this,arguments)}function Bf(e){return Xa.apply(this,arguments)}function Xa(){return Xa=Vt(Ee().m(function e(t){var n,r=arguments,a;return Ee().w(function(i){for(;;)switch(i.n){case 0:n=r.length>1&&r[1]!==void 0?r[1]:1,a=t.type,i.n=a==="canvas"?1:a==="svg"?2:a==="html"?3:4;break;case 1:return i.a(2,t.canvas);case 2:return i.a(2,Ms(t.svg,n));case 3:return i.a(2,Gf(t.html,t.css,t.width,t.height,n));case 4:return i.a(2)}},e)})),Xa.apply(this,arguments)}var $t=2048,Gs=(function(){function e(t,n,r){K(this,e),R(this,"cache",new Map),R(this,"pending",new Set),R(this,"atlas",{}),R(this,"glTexture",null),R(this,"dirty",!0),this.gl=t,this.renderers=n,this.scheduleRender=r,this.packCanvas=document.createElement("canvas"),this.packCanvas.width=$t,this.packCanvas.height=$t,this.packCtx=this.packCanvas.getContext("2d")}return Z(e,[{key:"renderAttachment",value:function(n,r,a){var i=this,o="".concat(n,":").concat(r);if(!(this.cache.has(o)||this.pending.has(o))){var s=this.renderers[r];if(s){var l=s(a);if(l){this.pending.add(o);var u=a.pixelRatio;Promise.resolve(l).then((function(){var c=Vt(Ee().m(function d(f){var h;return Ee().w(function(g){for(;;)switch(g.n){case 0:if(i.pending.has(o)){g.n=1;break}return g.a(2);case 1:if(i.pending.delete(o),f){g.n=2;break}return g.a(2);case 2:return g.n=3,Bf(f,u);case 3:if(h=g.v,!(!h||h.width===0||h.height===0)){g.n=4;break}return g.a(2);case 4:i.cache.set(o,{image:h,width:h.width,height:h.height}),i.dirty=!0,i.scheduleRender();case 5:return g.a(2)}},d)}));return function(d){return c.apply(this,arguments)}})())}}}}},{key:"regenerateAtlas",value:function(){if(this.dirty){this.dirty=!1;var n=[];if(this.cache.forEach(function(u,c){n.push({key:c,width:u.width,height:u.height,draw:function(f,h,g){f.drawImage(u.image,h,g)}})}),n.length===0){this.atlas={},this.deleteGLTexture();return}var r={x:0,y:0,rowHeight:0,maxRowWidth:0};this.packCtx.clearRect(0,0,$t,$t);var a=Is(n,this.packCtx,r),i=a.atlas,o=a.remaining;this.atlas=i,o.length>0&&console.warn("Sigma: ".concat(o.length," label attachment(s) could not fit in the ").concat($t,"x").concat($t," atlas and will not be rendered.")),this.deleteGLTexture();var s=this.gl,l=s.createTexture();s.activeTexture(s.TEXTURE0+xn),s.bindTexture(s.TEXTURE_2D,l),s.pixelStorei(s.UNPACK_PREMULTIPLY_ALPHA_WEBGL,!0),s.texImage2D(s.TEXTURE_2D,0,s.RGBA,s.RGBA,s.UNSIGNED_BYTE,this.packCanvas),s.pixelStorei(s.UNPACK_PREMULTIPLY_ALPHA_WEBGL,!1),s.texParameteri(s.TEXTURE_2D,s.TEXTURE_MIN_FILTER,s.LINEAR),s.texParameteri(s.TEXTURE_2D,s.TEXTURE_MAG_FILTER,s.LINEAR),s.texParameteri(s.TEXTURE_2D,s.TEXTURE_WRAP_S,s.CLAMP_TO_EDGE),s.texParameteri(s.TEXTURE_2D,s.TEXTURE_WRAP_T,s.CLAMP_TO_EDGE),this.glTexture=l}}},{key:"bindTexture",value:function(n){if(this.glTexture){var r=this.gl;r.activeTexture(r.TEXTURE0+n),r.bindTexture(r.TEXTURE_2D,this.glTexture)}}},{key:"getEntry",value:function(n,r){var a="".concat(n,":").concat(r);return this.atlas[a]||null}},{key:"invalidateNode",value:function(n){var r="".concat(n,":"),a=G(this.cache.keys()),i;try{for(a.s();!(i=a.n()).done;){var o=i.value;o.startsWith(r)&&(this.cache.delete(o),this.dirty=!0)}}catch(c){a.e(c)}finally{a.f()}var s=G(this.pending),l;try{for(s.s();!(l=s.n()).done;){var u=l.value;u.startsWith(r)&&this.pending.delete(u)}}catch(c){s.e(c)}finally{s.f()}}},{key:"restore",value:function(){this.glTexture=null,this.dirty=!0}},{key:"clear",value:function(){this.cache.clear(),this.pending.clear(),this.atlas={},this.dirty=!0,this.deleteGLTexture()}},{key:"kill",value:function(){this.clear(),this.packCanvas=null,this.packCtx=null,this.gl=null}},{key:"deleteGLTexture",value:function(){this.glTexture&&(this.gl.deleteTexture(this.glTexture),this.glTexture=null)}}])})(),Ya=(function(){function e(t){K(this,e),R(this,"buckets",new Map),R(this,"keyDepth",new Map);var n=G(t),r;try{for(n.s();!(r=n.n()).done;){var a=r.value;this.buckets.set(a,new Set)}}catch(i){n.e(i)}finally{n.f()}}return Z(e,[{key:"has",value:function(n){return this.keyDepth.has(n)}},{key:"getBucket",value:function(n){return this.buckets.get(n)}},{key:"set",value:function(n,r){var a,i=this.keyDepth.get(n);if(i!==r){var o=this.buckets.get(r);if(!o)throw new Error('Sigma: "'.concat(r,'" is not a declared depth layer'));i!==void 0&&((a=this.buckets.get(i))===null||a===void 0||a.delete(n)),o.add(n),this.keyDepth.set(n,r)}}},{key:"remove",value:function(n){var r=this.keyDepth.get(n);r!==void 0&&(this.buckets.get(r).delete(n),this.keyDepth.delete(n))}},{key:"clearAll",value:function(){var n=G(this.buckets.values()),r;try{for(n.s();!(r=n.n()).done;){var a=r.value;a.clear()}}catch(i){n.e(i)}finally{n.f()}this.keyDepth.clear()}},{key:"getSorted",value:function(n,r){var a=this.buckets.get(n);return!a||a.size===0?[]:$(a).sort(function(i,o){return r(i)-r(o)})}}])})(),Bs=(function(e){function t(n,r){return K(this,t),ae(this,t,[n,2,r])}return ie(t,e),Z(t,[{key:"updateNode",value:function(r,a,i,o,s,l,u,c){var d=this.indexMap.get(r);if(d===void 0)throw new Error('Node "'.concat(r,'" not allocated in NodeDataTexture'));var f=Ze(c),h=Q(f,4),g=h[0],m=h[1],v=h[2],p=h[3],b=d*2*4;this.data[b]=a,this.data[b+1]=i,this.data[b+2]=o,this.data[b+3]=s,this.data[b+4]=l,this.data[b+5]=u,this.data[b+6]=g*65536+m*256+v,this.data[b+7]=p/255,this.markDirty(d)}}])})(_n),Wf=1024,Of=1.5,Uf=4096,Ka=(function(){function e(t,n){var r;if(K(this,e),R(this,"texture",null),R(this,"framebuffer",null),!t.getExtension("EXT_color_buffer_float"))throw new Error("sigma: EXT_color_buffer_float is required for label/edge placement but is unavailable in this WebGL2 context.");this.gl=t,this.channels=n.channels,this.capacity=this.roundUpToPowerOfTwo((r=n.initialCapacity)!==null&&r!==void 0?r:Wf);var a=this.computeDimensions(this.capacity);this.textureWidth=a.width,this.textureHeight=a.height,this.create()}return Z(e,[{key:"roundUpToPowerOfTwo",value:function(n){return Math.pow(2,Math.ceil(Math.log2(Math.max(1,n))))}},{key:"computeDimensions",value:function(n){var r=Math.min(n,Uf),a=Math.ceil(n/r);return{width:r,height:a}}},{key:"create",value:function(){var n=this.gl,r=this.channels===1?n.R32F:n.RGBA32F,a=this.channels===1?n.RED:n.RGBA;if(this.texture=n.createTexture(),n.activeTexture(n.TEXTURE0),n.bindTexture(n.TEXTURE_2D,this.texture),n.texImage2D(n.TEXTURE_2D,0,r,this.textureWidth,this.textureHeight,0,a,n.FLOAT,null),n.texParameteri(n.TEXTURE_2D,n.TEXTURE_MIN_FILTER,n.NEAREST),n.texParameteri(n.TEXTURE_2D,n.TEXTURE_MAG_FILTER,n.NEAREST),n.texParameteri(n.TEXTURE_2D,n.TEXTURE_WRAP_S,n.CLAMP_TO_EDGE),n.texParameteri(n.TEXTURE_2D,n.TEXTURE_WRAP_T,n.CLAMP_TO_EDGE),this.framebuffer=n.createFramebuffer(),n.bindFramebuffer(n.FRAMEBUFFER,this.framebuffer),n.framebufferTexture2D(n.FRAMEBUFFER,n.COLOR_ATTACHMENT0,n.TEXTURE_2D,this.texture,0),n.checkFramebufferStatus(n.FRAMEBUFFER)!==n.FRAMEBUFFER_COMPLETE)throw new Error("sigma: float framebuffer for frame-pass placement is incomplete.");n.bindFramebuffer(n.FRAMEBUFFER,null),n.bindTexture(n.TEXTURE_2D,null)}},{key:"ensureCapacity",value:function(n){if(!(n<=this.capacity)){var r=this.gl;this.capacity=this.roundUpToPowerOfTwo(Math.ceil(n*Of));var a=this.computeDimensions(this.capacity);this.textureWidth=a.width,this.textureHeight=a.height,this.texture&&r.deleteTexture(this.texture),this.framebuffer&&r.deleteFramebuffer(this.framebuffer),this.create()}}},{key:"bindAsRenderTarget",value:function(){var n=this.gl;n.bindFramebuffer(n.FRAMEBUFFER,this.framebuffer),n.viewport(0,0,this.textureWidth,this.textureHeight)}},{key:"bind",value:function(n){var r=this.gl;r.activeTexture(r.TEXTURE0+n),r.bindTexture(r.TEXTURE_2D,this.texture)}},{key:"getTexture",value:function(){return this.texture}},{key:"getTextureWidth",value:function(){return this.textureWidth}},{key:"getTextureHeight",value:function(){return this.textureHeight}},{key:"restore",value:function(){this.gl.getExtension("EXT_color_buffer_float"),this.create()}},{key:"kill",value:function(){var n=this.gl;this.texture&&(n.deleteTexture(this.texture),this.texture=null),this.framebuffer&&(n.deleteFramebuffer(this.framebuffer),this.framebuffer=null)}}])})(),Ws=(function(e){function t(n,r){return K(this,t),ae(this,t,[n,2,r])}return ie(t,e),Z(t,[{key:"updateEdge",value:function(r,a,i,o,s,l){var u=arguments.length>6&&arguments[6]!==void 0?arguments[6]:0,c=arguments.length>7&&arguments[7]!==void 0?arguments[7]:0,d=arguments.length>8&&arguments[8]!==void 0?arguments[8]:0,f=this.indexMap.get(r);if(f===void 0)throw new Error('Edge "'.concat(r,'" not allocated in EdgeDataTexture'));var h=f*this.TEXELS_PER_ITEM*4;this.data[h+0]=a,this.data[h+1]=i,this.data[h+2]=o,this.data[h+3]=0,this.data[h+4]=s,this.data[h+5]=l,this.data[h+6]=u,this.data[h+7]=(c&15)<<4|d&15,this.markDirty(f)}}])})(_n);var qb=Ne(dt()),Xb=Ne(Bt());function Hf(e,t){if(e==null)return{};var n={};for(var r in e)if({}.hasOwnProperty.call(e,r)){if(t.indexOf(r)!==-1)continue;n[r]=e[r]}return n}function Os(e,t){if(e==null)return{};var n,r,a=Hf(e,t);if(Object.getOwnPropertySymbols){var i=Object.getOwnPropertySymbols(e);for(r=0;r<i.length;r++)n=i[r],t.indexOf(n)===-1&&{}.propertyIsEnumerable.call(e,n)&&(a[n]=e[n])}return a}var jf=["factor"],$f=["factor"],Us=1.5,Za={x:.5,y:.5,angle:0,ratio:1},Hs=(function(e){function t(){var n;K(this,t);for(var r=arguments.length,a=new Array(r),i=0;i<r;i++)a[i]=arguments[i];return n=ae(this,t,[].concat(a)),R(n,"minRatio",null),R(n,"maxRatio",null),R(n,"enabled",!0),R(n,"enabledZooming",!0),R(n,"enabledPanning",!0),R(n,"enabledRotation",!0),R(n,"constrainState",null),R(n,"state",B({},Za)),R(n,"previousState",B({},Za)),R(n,"nextFrame",null),R(n,"currentAnimationFrom",null),R(n,"currentAnimationTo",null),R(n,"animationCallback",null),n}return ie(t,e),Z(t,[{key:"x",get:function(){return this.state.x}},{key:"y",get:function(){return this.state.y}},{key:"angle",get:function(){return this.state.angle}},{key:"ratio",get:function(){return this.state.ratio}},{key:"getState",value:function(){return B({},this.state)}},{key:"getPreviousState",value:function(){return B({},this.previousState)}},{key:"getBoundedRatio",value:function(r){var a=r;return typeof this.minRatio=="number"&&(a=Math.max(a,this.minRatio)),typeof this.maxRatio=="number"&&(a=Math.min(a,this.maxRatio)),a}},{key:"validateState",value:function(r){var a=this.getState();return this.enabledPanning&&typeof r.x=="number"&&(a.x=r.x),this.enabledPanning&&typeof r.y=="number"&&(a.y=r.y),this.enabledZooming&&typeof r.ratio=="number"&&(a.ratio=this.getBoundedRatio(r.ratio)),this.enabledRotation&&typeof r.angle=="number"&&(a.angle=r.angle),this.constrainState?this.constrainState(a):a}},{key:"isAnimating",value:function(){return this.nextFrame!==null}},{key:"setState",value:function(r){if(!this.enabled)return this;var a=this.validateState(r);return Co(this.state,a)?this:(this.previousState=this.state,this.state=a,this.emit("updated",this.getState()),this)}},{key:"updateState",value:function(r){return this.setState(r(this.getState())),this}},{key:"animate",value:function(r,a){var i=this;return this.enabled?new Promise(function(o){return i.runAnimation(r,B(B({},Fo),a),o)}):Promise.resolve()}},{key:"cancelAnimation",value:function(){return this.nextFrame!==null&&(cancelAnimationFrame(this.nextFrame),this.nextFrame=null),this.resolveAnimation(),this.emitAnimationEnd(!1),this}},{key:"zoomIn",value:function(){var r=arguments.length>0&&arguments[0]!==void 0?arguments[0]:{},a=r.factor,i=a===void 0?Us:a,o=Os(r,jf);return this.animate({ratio:this.ratio/i},o)}},{key:"zoomOut",value:function(){var r=arguments.length>0&&arguments[0]!==void 0?arguments[0]:{},a=r.factor,i=a===void 0?Us:a,o=Os(r,$f);return this.animate({ratio:this.ratio*i},o)}},{key:"reset",value:function(r){return this.animate(Za,r)}},{key:"runAnimation",value:function(r,a,i){var o=this,s=or(a.easing),l=Date.now(),u=this.getState(),c=this.validateState(r),d=function(){if(!o.enabled){o.cancelAnimation();return}var h=a.duration>0?(Date.now()-l)/a.duration:1;if(h>=1){o.nextFrame=null,o.setState(c),o.resolveAnimation(),o.emitAnimationEnd(!0);return}var g=s(h);o.setState({x:u.x+(c.x-u.x)*g,y:u.y+(c.y-u.y)*g,angle:u.angle+(c.angle-u.angle)*g,ratio:u.ratio+(c.ratio-u.ratio)*g}),o.nextFrame=requestAnimationFrame(d)};this.cancelAnimation(),this.currentAnimationFrom=u,this.currentAnimationTo=c,this.animationCallback=i,this.emit("animationStart",{from:u,to:c}),d()}},{key:"resolveAnimation",value:function(){var r=this.animationCallback;this.animationCallback=null,r&&r()}},{key:"emitAnimationEnd",value:function(r){var a=this.currentAnimationFrom,i=this.currentAnimationTo;!a||!i||(this.currentAnimationFrom=null,this.currentAnimationTo=null,this.emit("animationEnd",{from:a,to:i,completed:r}))}}],[{key:"from",value:function(r){var a=new t;return a.setState(r)}}])})(cr);function Se(e,t){var n=t.getBoundingClientRect();return{x:e.clientX-n.left,y:e.clientY-n.top}}function We(e,t){var n=B(B({},Se(e,t)),{},{sigmaDefaultPrevented:!1,preventSigmaDefault:function(){n.sigmaDefaultPrevented=!0},original:e});return n}function _t(e){var t="x"in e?e:B(B({},e.touches[0]||e.previousTouches[0]),{},{original:e.original,sigmaDefaultPrevented:e.sigmaDefaultPrevented,preventSigmaDefault:function(){e.sigmaDefaultPrevented=!0,t.sigmaDefaultPrevented=!0}});return t}function Vf(e,t){var n=We(e,t);return n.delta=al(e),n}var qf=2;function Nr(e){for(var t=[],n=0,r=Math.min(e.length,qf);n<r;n++)t.push(e[n]);return t}function En(e,t,n){var r={touches:Nr(e.touches).map(function(a){return Se(a,n)}),previousTouches:t.map(function(a){return Se(a,n)}),sigmaDefaultPrevented:!1,preventSigmaDefault:function(){r.sigmaDefaultPrevented=!0},original:e};return r}function al(e){if(typeof e.deltaY<"u")return e.deltaY*-3/360;if(typeof e.detail<"u")return e.detail/-9;throw new Error("Captor: could not extract delta from event.")}var il=(function(e){function t(n,r){var a;return K(this,t),a=ae(this,t),a.container=n,a.renderer=r,a}return ie(t,e),Z(t)})(cr),Xf=["doubleClickTimeout","doubleClickZoomingDuration","doubleClickZoomingRatio","dragTimeout","draggedEventsTolerance","enableCameraMouseRotation","gestureTarget","inertiaDuration","inertiaRatio","zoomDuration","zoomingRatio"],Yf=Xf.reduce(function(e,t){return B(B({},e),{},R({},t,fr[t]))},{});function Sn(e){return e instanceof PointerEvent&&e.pointerType==="touch"}var Kf=(function(e){function t(n,r){var a;return K(this,t),a=ae(this,t,[n,r]),R(a,"enabled",!0),R(a,"draggedEvents",0),R(a,"downStartTime",null),R(a,"lastMouseX",null),R(a,"lastMouseY",null),R(a,"isMouseDown",!1),R(a,"isMoving",!1),R(a,"isPanningStage",!1),R(a,"movingTimeout",null),R(a,"startCameraState",null),R(a,"clicks",0),R(a,"doubleClickTimeout",null),R(a,"isRightMouseDown",!1),R(a,"startRotationAngle",null),R(a,"startCameraAngle",null),R(a,"currentWheelDirection",0),R(a,"lastWheelAnimationId",0),R(a,"settings",Yf),a.handleRightClick=a.handleRightClick.bind(a),a.handleDown=a.handleDown.bind(a),a.handleUp=a.handleUp.bind(a),a.handleMove=a.handleMove.bind(a),a.handleWheel=a.handleWheel.bind(a),a.handleLeave=a.handleLeave.bind(a),a.handleEnter=a.handleEnter.bind(a),n.addEventListener("contextmenu",a.handleRightClick,{capture:!1}),n.addEventListener("pointerdown",a.handleDown,{capture:!1}),n.addEventListener("wheel",a.handleWheel,{capture:!1,passive:!1}),n.addEventListener("pointerleave",a.handleLeave,{capture:!1}),n.addEventListener("pointerenter",a.handleEnter,{capture:!1}),document.addEventListener("pointermove",a.handleMove,{capture:!1}),document.addEventListener("pointerup",a.handleUp,{capture:!1}),a}return ie(t,e),Z(t,[{key:"kill",value:function(){var r=this.container;r.removeEventListener("contextmenu",this.handleRightClick),r.removeEventListener("pointerdown",this.handleDown),r.removeEventListener("wheel",this.handleWheel),r.removeEventListener("pointerleave",this.handleLeave),r.removeEventListener("pointerenter",this.handleEnter),document.removeEventListener("pointermove",this.handleMove),document.removeEventListener("pointerup",this.handleUp)}},{key:"handleClick",value:function(r){var a=this;if(this.enabled){if(this.clicks++,this.clicks===2)return this.clicks=0,typeof this.doubleClickTimeout=="number"&&(clearTimeout(this.doubleClickTimeout),this.doubleClickTimeout=null),this.handleDoubleClick(r);setTimeout(function(){a.clicks=0,a.doubleClickTimeout=null},this.settings.doubleClickTimeout),this.draggedEvents<this.settings.draggedEventsTolerance&&this.emit("click",We(r,this.container))}}},{key:"handleRightClick",value:function(r){this.enabled&&(this.settings.enableCameraMouseRotation&&r.preventDefault(),this.emit("rightClick",We(r,this.container)))}},{key:"handleDoubleClick",value:function(r){if(this.enabled){r.preventDefault(),r.stopPropagation();var a=We(r,this.container);if(this.emit("doubleClick",a),!a.sigmaDefaultPrevented){var i=this.renderer.getCamera(),o=i.getBoundedRatio(i.getState().ratio/this.settings.doubleClickZoomingRatio);i.animate(this.renderer.getViewportZoomedState(Se(r,this.container),o),{easing:"quadraticInOut",duration:this.settings.doubleClickZoomingDuration})}}}},{key:"handleDown",value:function(r){if(!(!this.enabled||Sn(r))){if(r.button===0){this.startCameraState=this.renderer.getCamera().getState();var a=Se(r,this.container),i=a.x,o=a.y;this.lastMouseX=i,this.lastMouseY=o,this.draggedEvents=0,this.downStartTime=Date.now(),this.isMouseDown=!0}if(r.button===2&&this.settings.enableCameraMouseRotation){var s=Se(r,this.container),l=s.x,u=s.y,c=this.container.offsetWidth/2,d=this.container.offsetHeight/2;this.startRotationAngle=Math.atan2(u-d,l-c),this.startCameraAngle=this.renderer.getCamera().getState().angle,this.isRightMouseDown=!0}this.emit("mousedown",We(r,this.container))}}},{key:"handleUp",value:function(r){var a=this;if(!(!this.enabled||Sn(r)||!this.isMouseDown&&!this.isRightMouseDown)){if(this.isRightMouseDown){this.isRightMouseDown=!1,this.startRotationAngle=null,this.startCameraAngle=null,this.emit("mouseup",We(r,this.container));return}var i=this.renderer.getCamera();this.isMouseDown=!1,this.isPanningStage&&(this.isPanningStage=!1,this.renderer._setPanning(!1)),typeof this.movingTimeout=="number"&&(clearTimeout(this.movingTimeout),this.movingTimeout=null);var o=Se(r,this.container),s=o.x,l=o.y,u=i.getState(),c=i.getPreviousState();this.isMoving?i.animate({x:u.x+this.settings.inertiaRatio*(u.x-c.x),y:u.y+this.settings.inertiaRatio*(u.y-c.y)},{duration:this.settings.inertiaDuration,easing:"quadraticOut"}):(this.lastMouseX!==s||this.lastMouseY!==l)&&i.setState({x:u.x,y:u.y}),this.isMoving=!1,setTimeout(function(){var d=a.draggedEvents>0;a.draggedEvents=0;var f=a.renderer.getSetting("hideEdgesOnMove")||a.renderer.getSetting("hideLabelsOnMove");d&&f&&a.renderer.refresh()},0),this.emit("mouseup",We(r,this.container)),(r.target===this.container||r.composedPath()[0]===this.container)&&this.handleClick(r)}}},{key:"handleMove",value:function(r){var a=this;if(!(!this.enabled||Sn(r))){var i=We(r,this.container);if(this.emit("mousemovebody",i),(r.target===this.container||r.composedPath()[0]===this.container)&&this.emit("mousemove",i),this.isMouseDown&&this.draggedEvents++,!i.sigmaDefaultPrevented){if(this.isMouseDown){this.isMoving=!0,typeof this.movingTimeout=="number"&&clearTimeout(this.movingTimeout),this.movingTimeout=window.setTimeout(function(){a.movingTimeout=null,a.isMoving=!1},this.settings.dragTimeout);var o=this.renderer.getCamera(),s=Se(r,this.container),l=s.x,u=s.y,c=this.renderer.viewportToFramedGraph({x:this.lastMouseX,y:this.lastMouseY}),d=this.renderer.viewportToFramedGraph({x:l,y:u}),f=c.x-d.x,h=c.y-d.y,g=o.getState(),m=g.x+f,v=g.y+h;this.isPanningStage||(this.isPanningStage=!0,this.renderer._setPanning(!0)),o.setState({x:m,y:v}),this.lastMouseX=l,this.lastMouseY=u,r.preventDefault(),r.stopPropagation()}if(this.isRightMouseDown){var p=Se(r,this.container),b=p.x,y=p.y,_=this.container.offsetWidth/2,x=this.container.offsetHeight/2,T=Math.atan2(y-x,b-_),E=T-this.startRotationAngle,S=this.renderer.getCamera();S.setState({angle:this.startCameraAngle+E}),r.preventDefault(),r.stopPropagation()}}}}},{key:"handleLeave",value:function(r){Sn(r)||this.emit("mouseleave",We(r,this.container))}},{key:"handleEnter",value:function(r){Sn(r)||this.emit("mouseenter",We(r,this.container))}},{key:"handleWheel",value:function(r){var a=this,i=this.renderer.getCamera();if(!(!this.enabled||!i.enabledZooming)){var o=Vf(r,this.container);if(this.emit("wheel",o),!o.sigmaDefaultPrevented){var s=this.settings.gestureTarget;if(s!=="page"){if(s==="shared"&&!r.ctrlKey&&!r.metaKey){this.renderer._showGestureHint("wheel");return}var l=al(r);if(l){r.preventDefault(),r.stopPropagation();var u=i.getState().ratio,c=l>0?1/this.settings.zoomingRatio:this.settings.zoomingRatio,d=i.getBoundedRatio(u*c),f=l>0?1:-1,h=Date.now();if(u!==d&&!(this.currentWheelDirection===f&&this.lastWheelTriggerTime&&h-this.lastWheelTriggerTime<this.settings.zoomDuration/5)){var g=++this.lastWheelAnimationId;i.animate(this.renderer.getViewportZoomedState(Se(r,this.container),d),{easing:"quadraticOut",duration:this.settings.zoomDuration}).then(function(){a.lastWheelAnimationId===g&&(a.currentWheelDirection=0)}),this.currentWheelDirection=f,this.lastWheelTriggerTime=h}}}}}}},{key:"setSettings",value:function(r){this.settings=r}}])})(il),Zf=["dragTimeout","gestureTarget","inertiaDuration","inertiaRatio","doubleClickTimeout","doubleClickZoomingRatio","doubleClickZoomingDuration","tapMoveTolerance"],Jf=Zf.reduce(function(e,t){return B(B({},e),{},R({},t,fr[t]))},{}),Qf=(function(e){function t(n,r){var a;return K(this,t),a=ae(this,t,[n,r]),R(a,"enabled",!0),R(a,"isMoving",!1),R(a,"hasMoved",!1),R(a,"isPanningStage",!1),R(a,"isZoomingStage",!1),R(a,"touchMode",0),R(a,"startTouchesPositions",[]),R(a,"lastTouches",[]),R(a,"lastTap",null),R(a,"settings",Jf),a.handleStart=a.handleStart.bind(a),a.handleLeave=a.handleLeave.bind(a),a.handleMove=a.handleMove.bind(a),n.addEventListener("touchstart",a.handleStart,{capture:!1}),n.addEventListener("touchcancel",a.handleLeave,{capture:!1}),document.addEventListener("touchend",a.handleLeave,{capture:!1,passive:!1}),document.addEventListener("touchmove",a.handleMove,{capture:!1,passive:!1}),a}return ie(t,e),Z(t,[{key:"kill",value:function(){var r=this.container;r.removeEventListener("touchstart",this.handleStart),r.removeEventListener("touchcancel",this.handleLeave),document.removeEventListener("touchend",this.handleLeave),document.removeEventListener("touchmove",this.handleMove)}},{key:"getDimensions",value:function(){return{width:this.container.offsetWidth,height:this.container.offsetHeight}}},{key:"doesCapture",value:function(r){var a=this.settings.gestureTarget;return a==="graph"?!0:a==="page"?!1:r>=2}},{key:"syncStageFlags",value:function(){var r=this.touchMode===1&&this.hasMoved,a=this.touchMode===2;r!==this.isPanningStage&&(this.isPanningStage=r,this.renderer._setPanning(r)),a!==this.isZoomingStage&&(this.isZoomingStage=a,this.renderer._setZooming(a))}},{key:"handleStart",value:function(r){var a=this;if(this.enabled){var i=Nr(r.touches);if(this.touchMode=i.length,this.startCameraState=this.renderer.getCamera().getState(),this.startTouchesPositions=i.map(function(h){return Se(h,a.container)}),this.touchMode===2){var o=Q(this.startTouchesPositions,2),s=o[0],l=s.x,u=s.y,c=o[1],d=c.x,f=c.y;this.startTouchesAngle=Math.atan2(f-u,d-l),this.startTouchesDistance=Math.sqrt(Math.pow(d-l,2)+Math.pow(f-u,2))}this.syncStageFlags(),this.emit("touchdown",En(r,this.lastTouches,this.container)),this.lastTouches=i,this.lastTouchesPositions=this.startTouchesPositions,r.cancelable&&(this.doesCapture(r.touches.length)||this.renderer._hasNodeDrag())&&r.preventDefault()}}},{key:"handleLeave",value:function(r){if(!(!this.enabled||!this.startTouchesPositions.length)){switch(r.cancelable&&this.doesCapture(this.touchMode)&&r.preventDefault(),this.movingTimeout&&(this.isMoving=!1,clearTimeout(this.movingTimeout)),this.touchMode){case 2:if(r.touches.length===1){this.handleStart(r);break}case 1:if(this.isMoving){var a=this.renderer.getCamera(),i=a.getState(),o=a.getPreviousState();a.animate({x:i.x+this.settings.inertiaRatio*(i.x-o.x),y:i.y+this.settings.inertiaRatio*(i.y-o.y)},{duration:this.settings.inertiaDuration,easing:"quadraticOut"})}this.hasMoved=!1,this.isMoving=!1,this.touchMode=0;break}if(this.syncStageFlags(),this.emit("touchup",En(r,this.lastTouches,this.container)),!r.touches.length){var s=Se(this.lastTouches[0],this.container),l=this.startTouchesPositions[0],u=Math.pow(s.x-l.x,2)+Math.pow(s.y-l.y,2);if(!r.touches.length&&u<Math.pow(this.settings.tapMoveTolerance,2))if(this.lastTap&&Date.now()-this.lastTap.time<this.settings.doubleClickTimeout){var c=En(r,this.lastTouches,this.container);if(this.emit("doubletap",c),this.lastTap=null,!c.sigmaDefaultPrevented&&this.settings.gestureTarget!=="page"){var d=this.renderer.getCamera(),f=d.getBoundedRatio(d.getState().ratio/this.settings.doubleClickZoomingRatio);d.animate(this.renderer.getViewportZoomedState(s,f),{easing:"quadraticInOut",duration:this.settings.doubleClickZoomingDuration})}}else{var h=En(r,this.lastTouches,this.container);this.emit("tap",h),this.lastTap={time:Date.now(),position:h.touches[0]||h.previousTouches[0]}}}this.lastTouches=Nr(r.touches),this.startTouchesPositions=[]}}},{key:"handleMove",value:function(r){var a=this;if(!(!this.enabled||!this.startTouchesPositions.length)){var i=this.doesCapture(r.touches.length);i&&r.preventDefault();var o=Nr(r.touches),s=o.map(function(U){return Se(U,a.container)}),l=this.lastTouches;this.lastTouches=o,this.lastTouchesPositions=s;var u=En(r,l,this.container);if(this.emit("touchmove",u),!u.sigmaDefaultPrevented&&(this.hasMoved||(this.hasMoved=s.some(function(U,Y){var q=a.startTouchesPositions[Y];return q&&(U.x!==q.x||U.y!==q.y)})),!!this.hasMoved)){if(!i){this.settings.gestureTarget==="shared"&&this.renderer._showGestureHint("touch");return}this.isMoving=!0,this.syncStageFlags(),this.movingTimeout&&clearTimeout(this.movingTimeout),this.movingTimeout=window.setTimeout(function(){a.isMoving=!1},this.settings.dragTimeout);var c=this.renderer.getCamera(),d=this.startCameraState,f=this.renderer.getSetting("stagePadding");switch(this.touchMode){case 1:{var h=this.renderer.viewportToFramedGraph((this.startTouchesPositions||[])[0]),g=h.x,m=h.y,v=this.renderer.viewportToFramedGraph(s[0]),p=v.x,b=v.y;c.setState({x:d.x+g-p,y:d.y+m-b});break}case 2:{var y={x:.5,y:.5,angle:0,ratio:1},_=s[0],x=_.x,T=_.y,E=s[1],S=E.x,w=E.y,A=Math.atan2(w-T,S-x)-this.startTouchesAngle,C=Math.hypot(w-T,S-x)/this.startTouchesDistance,D=c.getBoundedRatio(d.ratio/C);y.ratio=D,y.angle=d.angle+A;var F=this.getDimensions(),k=this.renderer.viewportToFramedGraph((this.startTouchesPositions||[])[0],{cameraState:d}),L=Math.min(F.width,F.height)-2*f,z=L/F.width,N=L/F.height,P=D/L,I=x-L/2/z,M=T-L/2/N,j=er({x:I,y:M},y.angle);I=j.x,M=j.y,y.x=k.x-I*P,y.y=k.y+M*P,c.setState(y);break}}}}}},{key:"setSettings",value:function(r){this.settings=r}}])})(il),eg=(function(){function e(t,n,r,a){K(this,e),R(this,"pendingNode",null),R(this,"session",null),this.graph=t,this.viewportToGraph=n,this.setNodesState=r,this.emit=a}return Z(e,[{key:"start",value:function(n,r,a,i,o){var s=a(n),l=!1;if(this.emit("nodeDragStart",{node:n,allDraggedNodes:s,event:r,preventSigmaDefault:function(){l=!0}}),l)return!1;var u=new Map,c=G(s),d;try{for(c.s();!(d=c.n()).done;){var f=d.value;u.set(f,{x:this.graph.getNodeAttribute(f,i),y:this.graph.getNodeAttribute(f,o)})}}catch(h){c.e(h)}finally{c.f()}return this.session={node:n,allNodes:s,startPosition:this.viewportToGraph(r),startNodePositions:u,xAttr:i,yAttr:o},this.setNodesState(s,{isDragged:!0}),!0}},{key:"applyMove",value:function(n,r){var a=this.session,i=a.allNodes,o=a.startNodePositions,s=a.startPosition,l=a.xAttr,u=a.yAttr,c=this.viewportToGraph(n),d={x:c.x-s.x,y:c.y-s.y},f=G(i),h;try{for(f.s();!(h=f.n()).done;){var g=h.value,m=o.get(g);if(!(!m||!this.graph.hasNode(g))){var v={x:m.x+d.x,y:m.y+d.y};r?this.graph.mergeNodeAttributes(g,r(v,g)):(this.graph.setNodeAttribute(g,l,v.x),this.graph.setNodeAttribute(g,u,v.y))}}}catch(p){f.e(p)}finally{f.f()}}},{key:"end",value:function(){if(this.pendingNode=null,!this.session)return null;var n=this.session,r=n.node,a=n.allNodes;return this.setNodesState(a,{isDragged:!1}),this.session=null,{node:r,allNodes:a}}},{key:"removeNode",value:function(n){n===this.pendingNode&&(this.pendingNode=null),this.session&&(this.session.node===n?(this.setNodesState(this.session.allNodes,{isDragged:!1}),this.session=null):this.session.allNodes.includes(n)&&(this.session.allNodes=this.session.allNodes.filter(function(r){return r!==n}),this.session.startNodePositions.delete(n)))}},{key:"clear",value:function(){this.pendingNode=null,this.session=null}}])})();function js(e,t){return e<t?"".concat(e,"\0").concat(t):"".concat(t,"\0").concat(e)}var tg=(function(){function e(t,n){K(this,e),R(this,"groups",new Map),R(this,"edgeToGroupKey",new Map),this.graph=t,this.onGroupChanged=n}return Z(e,[{key:"getGroupKey",value:function(n){return js(this.graph.source(n),this.graph.target(n))}},{key:"sortGroup",value:function(n,r){var a=this,i=r.split("\0")[0];n.sort(function(o,s){var l=a.graph.source(o)===i||!a.graph.isDirected(o)?0:1,u=a.graph.source(s)===i||!a.graph.isDirected(s)?0:1;return l-u})}},{key:"register",value:function(n){var r=this.getGroupKey(n),a=this.groups.get(r);a||(a=[],this.groups.set(r,a)),a.includes(n)||a.push(n),this.edgeToGroupKey.set(n,r),a.length>1&&this.sortGroup(a,r),this.onGroupChanged(a,a.length)}},{key:"unregister",value:function(n){var r=this.edgeToGroupKey.get(n);if(r){this.edgeToGroupKey.delete(n);var a=this.groups.get(r);if(a){var i=a.indexOf(n);i!==-1&&a.splice(i,1),a.length===0?this.groups.delete(r):this.onGroupChanged(a,a.length)}}}},{key:"getGroup",value:function(n){var r,a=this.edgeToGroupKey.get(n);return a?(r=this.groups.get(a))!==null&&r!==void 0?r:[]:[]}},{key:"getSiblings",value:function(n){return this.getGroup(n).filter(function(r){return r!==n})}},{key:"rebuild",value:function(){var n=this;this.groups.clear(),this.edgeToGroupKey.clear(),this.graph.forEachEdge(function(l,u,c,d){var f=js(c,d),h=n.groups.get(f);h||(h=[],n.groups.set(f,h)),h.push(l),n.edgeToGroupKey.set(l,f)});var r=G(this.groups),a;try{for(r.s();!(a=r.n()).done;){var i=Q(a.value,2),o=i[0],s=i[1];s.length>1&&this.sortGroup(s,o),this.onGroupChanged(s,s.length)}}catch(l){r.e(l)}finally{r.f()}}},{key:"clear",value:function(){this.groups.clear(),this.edgeToGroupKey.clear()}}])})();function $s(e,t){var n;if(e===!1||e==="extend"||e==="separate")return e;var r=e[t];return r!==void 0?r:(n=e.default)!==null&&n!==void 0?n:!1}function Vs(e){return e===!1?!1:e==="extend"||e==="separate"?!0:Object.values(e).some(function(t){return t==="extend"||t==="separate"})}function qs(e){return e==="separate"?!0:e===!1||e==="extend"?!1:e.default==="separate"?!0:Object.values(e).includes("separate")}var ge={node:{parent:null,eventSuffix:"Node",payloadKey:"node",isEnabled:function(){return!0},writesPickingThisFrame:function(){return!0},setHover:function(t,n,r){return t.setNodeState(n,{isHovered:r})},getCursor:function(t,n){var r;return(r=t.nodeDataCache[n])===null||r===void 0?void 0:r.cursor},isHitValid:function(t,n){var r;return((r=t.nodeDataCache[n])===null||r===void 0?void 0:r.visibility)!=="hidden"}},edge:{parent:null,eventSuffix:"Edge",payloadKey:"edge",isEnabled:function(t){return!!t.settings.enableEdgeEvents},writesPickingThisFrame:function(t){return!!t.settings.enableEdgeEvents},setHover:function(t,n,r){return t.setEdgeState(n,{isHovered:r})},getCursor:function(t,n){var r;return(r=t.edgeDataCache[n])===null||r===void 0?void 0:r.cursor}},nodeLabel:{parent:"node",eventSuffix:"NodeLabel",payloadKey:"node",isEnabled:function(t){return qs(t.settings.nodeLabelEvents)},writesPickingThisFrame:function(t){return Vs(t.settings.nodeLabelEvents)},setHover:function(t,n,r){return t.setNodeState(n,{isLabelHovered:r})},getCursor:function(t,n){var r;return(r=t.nodeDataCache[n])===null||r===void 0?void 0:r.labelCursor},resolveForVerb:function(t,n,r){var a=$s(r.settings.nodeLabelEvents,n);return a===!1?null:a==="extend"?{kind:"node",key:t.key}:t}},edgeLabel:{parent:"edge",eventSuffix:"EdgeLabel",payloadKey:"edge",isEnabled:function(t){return qs(t.settings.edgeLabelEvents)},writesPickingThisFrame:function(t){return Vs(t.settings.edgeLabelEvents)},setHover:function(t,n,r){return t.setEdgeState(n,{isLabelHovered:r})},getCursor:function(t,n){var r;return(r=t.edgeDataCache[n])===null||r===void 0?void 0:r.labelCursor},resolveForVerb:function(t,n,r){var a=$s(r.settings.edgeLabelEvents,n);return a===!1?null:a==="extend"?{kind:"edge",key:t.key}:t}}},Et=["node","edge","nodeLabel","edgeLabel"];function ng(){return{lookup:[null],idsByKind:{node:new Map,edge:new Map,nodeLabel:new Map,edgeLabel:new Map},nextId:1}}function Tt(e,t,n){var r;return(r=e.idsByKind[t].get(n))!==null&&r!==void 0?r:0}function Ja(e,t){var n=G(Et),r;try{for(n.s();!(r=n.n()).done;){var a=r.value;if(a===t||ge[a].parent===t){var i=G(e.idsByKind[a].values()),o;try{for(i.s();!(o=i.n()).done;){var s=o.value;e.lookup[s]=null}}catch(f){i.e(f)}finally{i.f()}e.idsByKind[a]=new Map}}}catch(f){n.e(f)}finally{n.f()}var l=1,u=G(Et),c;try{for(u.s();!(c=u.n()).done;){var d=c.value;if(d===t)break;l+=e.idsByKind[d].size}}catch(f){u.e(f)}finally{u.f()}e.nextId=l}function Xs(e,t,n){var r=e.nextId++;return e.idsByKind[t].set(n,r),e.lookup[r]={kind:t,key:n},r}function rg(e,t){var n=G(Et),r;try{for(n.s();!(r=n.n()).done;){var a=r.value;if(ge[a].parent!==null){var i=G(e.idsByKind[a].values()),o;try{for(i.s();!(o=i.n()).done;){var s=o.value;e.lookup[s]=null}}catch(x){i.e(x)}finally{i.f()}e.idsByKind[a]=new Map}}}catch(x){n.e(x)}finally{n.f()}e.nextId=1;var l=G(Et),u;try{for(l.s();!(u=l.n()).done;){var c=u.value;if(ge[c].parent===null){var d=G(e.idsByKind[c].values()),f;try{for(d.s();!(f=d.n()).done;){var h=f.value;h>=e.nextId&&(e.nextId=h+1)}}catch(x){d.e(x)}finally{d.f()}}}}catch(x){l.e(x)}finally{l.f()}var g=G(Et),m;try{for(g.s();!(m=g.n()).done;){var v=m.value,p=ge[v];if(!(p.parent===null||!p.isEnabled(t))){var b=G(e.idsByKind[p.parent].keys()),y;try{for(b.s();!(y=b.n()).done;){var _=y.value;e.idsByKind[v].set(_,e.nextId),e.lookup[e.nextId]={kind:v,key:_},e.nextId++}}catch(x){b.e(x)}finally{b.f()}}}}catch(x){g.e(x)}finally{g.f()}}function ag(e,t){var n=G(Et),r;try{for(n.s();!(r=n.n()).done;){var a=r.value,i=e[a],o=t[a];if(i!==o){if(i.size!==o.size)return!1;var s=G(i),l;try{for(s.s();!(l=s.n()).done;){var u=Q(l.value,2),c=u[0],d=u[1];if(o.get(c)!==d)return!1}}catch(f){s.e(f)}finally{s.f()}}}}catch(f){n.e(f)}finally{n.f()}return!0}function Ys(e,t,n){ge[t.kind].setHover(e,t.key,n)}function ig(e,t){return ge[t.kind].getCursor(e,t.key)}function Rn(e,t){return"".concat(t).concat(ge[e].eventSuffix)}function An(e,t){return B(B({},t),{},R({},ge[e.kind].payloadKey,e.key))}function og(e,t,n){var r=ge[t.kind].resolveForVerb;return r?r(t,n,e):t}function sg(e,t){var n=ge[t.kind].isHitValid;return n?n(e,t.key):!0}function Qa(e){return Et.filter(function(t){return t===e||ge[t].parent===e})}function ol(e,t,n){return t&&(t=og(e,t,n)),t&&!sg(e,t)&&(t=null),t}function ei(e,t,n){var r=n==="wheel"?e.stateManager.hovered:e.getHitAtPosition(t);return ol(e,r,n)}function lg(e,t){return!e||!t?e===t:e.kind===t.kind&&e.key===t.key}function sl(e,t,n){var r={event:n,preventSigmaDefault:function(){return n.preventSigmaDefault()}},a=e.dragManager.session,i=a?{kind:"node",key:a.node}:ol(e,t,"enter"),o=e.stateManager,s=o.hovered;lg(s,i)||(s&&(Ys(e,s,!1),e.emit(Rn(s.kind,"leave"),An(s,r))),o.setHovered(i),i&&(Ys(e,i,!0),e.emit(Rn(i.kind,"enter"),An(i,r))),e.updateContainerCursor())}function ug(e,t,n,r){r.handleResize=function(){return e.scheduleRefresh()},window.addEventListener("resize",r.handleResize),r.handleMove=function(i){e.hoverResolver.pointerMoved(_t(i))},r.handleMoveBody=function(i){var o=_t(i),s=e.dragManager;if(s.pendingNode&&!s.session){var l=e.nodeStyleAnalysis,u=l.xAttribute,c=l.yAttribute,d=e.settings;s.start(s.pendingNode,o,d.getDraggedNodes,u||"x",c||"y"),s.pendingNode=null}s.session&&(s.applyMove(o,e.settings.dragPositionToAttributes),e.emit("nodeDrag",{node:s.session.node,allDraggedNodes:s.session.allNodes,event:o}),o.preventSigmaDefault()),e.emit("moveBody",{event:o,preventSigmaDefault:function(){return o.preventSigmaDefault()}})},r.handleLeave=function(i){var o=_t(i),s={event:o,preventSigmaDefault:function(){return o.preventSigmaDefault()}};e.hoverResolver.pointerLeft(),sl(e,null,o),e.emit("leaveStage",s)},r.handleEnter=function(i){var o=_t(i);e.emit("enterStage",{event:o,preventSigmaDefault:function(){return o.preventSigmaDefault()}})};var a=function(o){return function(s){var l=_t(s),u={event:l,preventSigmaDefault:function(){return l.preventSigmaDefault()}},c=ei(e,l,o);if(c){e.emit(Rn(c.kind,o),An(c,u));return}e.emit("".concat(o,"Stage"),u)}};r.handleClick=a("click"),r.handleRightClick=a("rightClick"),r.handleDoubleClick=a("doubleClick"),r.handleWheel=a("wheel"),r.handleDown=function(i){var o=_t(i),s={event:o,preventSigmaDefault:function(){return o.preventSigmaDefault()}},l=ei(e,o,"down");if(l){l.kind==="node"&&e.settings.enableNodeDrag&&(e.dragManager.pendingNode=l.key),e.emit(Rn(l.kind,"down"),An(l,s));return}e.emit("downStage",s)},r.handleUp=function(i){var o=_t(i),s={event:o,preventSigmaDefault:function(){return o.preventSigmaDefault()}},l=e.dragManager.end();l&&e.emit("nodeDragEnd",B({node:l.node,allDraggedNodes:l.allNodes},s));var u=ei(e,o,"up");if(u){e.emit(Rn(u.kind,"up"),An(u,s));return}e.emit("upStage",s)},t.on("mousemove",r.handleMove),t.on("mousemovebody",r.handleMoveBody),t.on("click",r.handleClick),t.on("rightClick",r.handleRightClick),t.on("doubleClick",r.handleDoubleClick),t.on("wheel",r.handleWheel),t.on("mousedown",r.handleDown),t.on("mouseup",r.handleUp),t.on("mouseleave",r.handleLeave),t.on("mouseenter",r.handleEnter),n.on("touchdown",r.handleDown),n.on("touchdown",r.handleMove),n.on("touchup",r.handleUp),n.on("touchmove",r.handleMove),n.on("tap",r.handleClick),n.on("doubletap",r.handleDoubleClick),n.on("touchmove",r.handleMoveBody)}function cg(e,t){var n=e.graph,r=new Set(["x","y","zIndex","type"]);t.eachNodeAttributesUpdatedGraphUpdate=function(a){var i,o=(i=a.hints)===null||i===void 0?void 0:i.attributes,s=!o||o.some(function(l){return r.has(l)});e.refresh({partialGraph:{nodes:n.nodes()},skipIndexation:!s,schedule:!0})},t.eachEdgeAttributesUpdatedGraphUpdate=function(a){var i,o=(i=a.hints)===null||i===void 0?void 0:i.attributes,s=o&&["zIndex","type"].some(function(l){return o?.includes(l)});e.refresh({partialGraph:{edges:n.edges()},skipIndexation:!s,schedule:!0})},t.addNodeGraphUpdate=function(a){e.addNode(a.key),e.refresh({partialGraph:{nodes:[a.key]},skipIndexation:!1,schedule:!0})},t.updateNodeGraphUpdate=function(a){e.refresh({partialGraph:{nodes:[a.key]},skipIndexation:!1,schedule:!0})},t.dropNodeGraphUpdate=function(a){e.removeNode(a.key),e.refresh({schedule:!0})},t.addEdgeGraphUpdate=function(a){var i=a.key;e.edgeGroups.register(i),e.addEdge(i);var o=e.edgeGroups.getSiblings(i),s=G(o),l;try{for(s.s();!(l=s.n()).done;){var u=l.value;e.addEdge(u)}}catch(c){s.e(c)}finally{s.f()}e.refresh({partialGraph:{edges:[i].concat($(o))},schedule:!0})},t.updateEdgeGraphUpdate=function(a){e.refresh({partialGraph:{edges:[a.key]},skipIndexation:!1,schedule:!0})},t.dropEdgeGraphUpdate=function(a){var i=a.key,o=e.edgeGroups.getSiblings(i);e.edgeGroups.unregister(i),e.removeEdge(i);var s=G(o),l;try{for(s.s();!(l=s.n()).done;){var u=l.value;e.addEdge(u)}}catch(c){s.e(c)}finally{s.f()}e.refresh({schedule:!0})},t.clearEdgesGraphUpdate=function(){e.clearEdgeState(),e.clearEdgeIndices(),e.refresh({schedule:!0})},t.clearGraphUpdate=function(){e.clearEdgeState(),e.clearNodeState(),e.clearEdgeIndices(),e.clearNodeIndices(),e.refresh({schedule:!0})},n.on("nodeAdded",t.addNodeGraphUpdate),n.on("nodeDropped",t.dropNodeGraphUpdate),n.on("nodeAttributesUpdated",t.updateNodeGraphUpdate),n.on("eachNodeAttributesUpdated",t.eachNodeAttributesUpdatedGraphUpdate),n.on("edgeAdded",t.addEdgeGraphUpdate),n.on("edgeDropped",t.dropEdgeGraphUpdate),n.on("edgeAttributesUpdated",t.updateEdgeGraphUpdate),n.on("eachEdgeAttributesUpdated",t.eachEdgeAttributesUpdatedGraphUpdate),n.on("edgesCleared",t.clearEdgesGraphUpdate),n.on("cleared",t.clearGraphUpdate)}function dg(e,t){e.removeListener("nodeAdded",t.addNodeGraphUpdate),e.removeListener("nodeDropped",t.dropNodeGraphUpdate),e.removeListener("nodeAttributesUpdated",t.updateNodeGraphUpdate),e.removeListener("eachNodeAttributesUpdated",t.eachNodeAttributesUpdatedGraphUpdate),e.removeListener("edgeAdded",t.addEdgeGraphUpdate),e.removeListener("edgeDropped",t.dropEdgeGraphUpdate),e.removeListener("edgeAttributesUpdated",t.updateEdgeGraphUpdate),e.removeListener("eachEdgeAttributesUpdated",t.eachEdgeAttributesUpdatedGraphUpdate),e.removeListener("edgesCleared",t.clearEdgesGraphUpdate),e.removeListener("cleared",t.clearGraphUpdate)}var hg={x:.5,y:.5,angle:0,ratio:1},fg=8,gg=.001;function pg(e){var t=e.coords,n=e.nodeData,r=e.dimensions,a=e.stagePadding,i=e.zoomToSizeRatioFunction,o=e.itemSizesReference,s=e.fitLabels,l=e.nodeLabelBox,u=Object.keys(t);if(!u.length)return e.extent;for(var c=i(1)||1,d=o==="positions",f=r.width,h=r.height,g=e.extent,m=0;m<fg;m++){for(var v=hr(g),p={width:g.x[1]-g.x[0]||1,height:g.y[1]-g.y[0]||1},b=Ke(hg,r,p,a),y=Ks(b,v({x:0,y:0}),f,h),_=Ks(b,v({x:1,y:0}),f,h),x=Math.hypot(_.x-y.x,_.y-y.y)||1,T=1/0,E=-1/0,S=1/0,w=-1/0,A=0,C=u.length;A<C;A++){var D=u[A],F=n[D],k=F.size/c*(d?x:1),L=-k,z=k,N=-k,P=k;if(s){var I=l(F,k);I&&(I.minX<L&&(L=I.minX),I.maxX>z&&(z=I.maxX),I.minY<N&&(N=I.minY),I.maxY>P&&(P=I.maxY))}var M=t[D],j=M.x,U=M.y;T=Math.min(T,j+L/x),E=Math.max(E,j+z/x),S=Math.min(S,U-P/x),w=Math.max(w,U-N/x)}var Y={x:[T,E],y:[S,w]},q=Math.max(Y.x[1]-Y.x[0],Y.y[1]-Y.y[0])||1,X=Math.max(Math.abs(Y.x[0]-g.x[0]),Math.abs(Y.x[1]-g.x[1]),Math.abs(Y.y[0]-g.y[0]),Math.abs(Y.y[1]-g.y[1]));if(g=Y,X/q<gg)break}return g}function Ks(e,t,n,r){var a=ht(e,t);return{x:(1+a.x)*n/2,y:(1-a.y)*r/2}}var vg=typeof navigator<"u"&&/Mac|iP(hone|ad|od)/.test(navigator.platform),mg=1500,bg=`.sigma-gesture-hint {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  text-align: center;
  padding: 1em;
  background: #00000066;
  color: #ffffff;
  font-size: 1em;
  pointer-events: none;
  transition: opacity 0.1s;
}`;function Zs(e){var t,n,r=Wt("style");r.textContent=e,document.head.appendChild(r);var a=((t=(n=r.sheet)===null||n===void 0?void 0:n.cssRules.length)!==null&&t!==void 0?t:0)>0;return r.remove(),a}var xt=null;function yg(){return xt===null&&(xt=bg,Zs("@scope {}")&&(xt=`@scope {
`.concat(xt,`
}`)),Zs("@layer sigma-gesture-hint {}")&&(xt=`@layer sigma-gesture-hint {
`.concat(xt,`
}`))),xt}var _g=(function(){function e(t){K(this,e),R(this,"hideTimeout",null),this.styleElement=Wt("style"),this.styleElement.textContent=yg(),t.appendChild(this.styleElement),this.element=Wt("div",{opacity:"0"},{class:"sigma-gesture-hint"}),t.appendChild(this.element)}return Z(e,[{key:"show",value:function(n,r){var a=this,i=n==="touch"?r.sharedGestureTouchMessage:vg?r.sharedGestureAppleWheelMessage:r.sharedGestureWheelMessage;i&&(this.element.textContent=i,this.element.style.opacity="1",typeof this.hideTimeout=="number"&&clearTimeout(this.hideTimeout),this.hideTimeout=window.setTimeout(function(){a.hideTimeout=null,a.element.style.opacity="0"},mg))}},{key:"kill",value:function(){typeof this.hideTimeout=="number"&&clearTimeout(this.hideTimeout),this.styleElement.remove(),this.element.remove()}}])})(),xg=(function(){function e(t){K(this,e),R(this,"readBuffer",null),R(this,"fence",null),R(this,"result",new Uint8Array(4)),R(this,"generation",0),R(this,"readGeneration",0),R(this,"lastEvent",null),R(this,"active",!1),R(this,"dirty",!1),R(this,"rafId",null),R(this,"killed",!1),this.options=t}return Z(e,[{key:"pointerMoved",value:function(n){this.lastEvent=n,this.active=!0,!this.options.isRenderPending()&&this.request()}},{key:"pointerLeft",value:function(){this.active=!1,this.dirty=!1}},{key:"frameRendered",value:function(){this.request()}},{key:"invalidate",value:function(){this.generation++}},{key:"request",value:function(){if(!(this.killed||!this.active||!this.lastEvent)){if(this.fence){this.dirty=!0;return}this.startRead()}}},{key:"startRead",value:function(){var n=this.options,r=n.gl,a=n.getFrameBuffer,i=n.getPixelRatio,o=n.getDownSizingRatio;this.readGeneration=this.generation;var s=this.lastEvent,l=tr(r,s.x,s.y,i(),o()),u=Q(l,2),c=u[0],d=u[1];this.readBuffer||(this.readBuffer=r.createBuffer()),r.bindFramebuffer(r.FRAMEBUFFER,a()),r.bindBuffer(r.PIXEL_PACK_BUFFER,this.readBuffer),r.bufferData(r.PIXEL_PACK_BUFFER,4,r.STREAM_READ),r.readPixels(c,d,1,1,r.RGBA,r.UNSIGNED_BYTE,0),r.bindBuffer(r.PIXEL_PACK_BUFFER,null),r.bindFramebuffer(r.FRAMEBUFFER,null),this.fence=r.fenceSync(r.SYNC_GPU_COMMANDS_COMPLETE,0),r.flush(),this.schedulePoll(s)}},{key:"schedulePoll",value:function(n){var r=this;this.rafId=requestAnimationFrame(function(){r.rafId=null,r.poll(n)})}},{key:"poll",value:function(n){var r=this.options,a=r.gl,i=r.onIndex;if(!(this.killed||!this.fence)){var o=a.clientWaitSync(this.fence,0,0);if(o===a.TIMEOUT_EXPIRED){this.schedulePoll(n);return}if(a.deleteSync(this.fence),this.fence=null,o!==a.WAIT_FAILED&&this.readGeneration===this.generation){a.bindBuffer(a.PIXEL_PACK_BUFFER,this.readBuffer),a.getBufferSubData(a.PIXEL_PACK_BUFFER,0,this.result),a.bindBuffer(a.PIXEL_PACK_BUFFER,null);var s=Q(this.result,4),l=s[0],u=s[1],c=s[2],d=s[3];this.active&&i(nr(l,u,c,d),n)}this.dirty&&(this.dirty=!1,this.active&&this.lastEvent&&!this.options.isRenderPending()&&this.startRead())}}},{key:"reset",value:function(){this.rafId!==null&&cancelAnimationFrame(this.rafId),this.rafId=null,this.fence=null,this.readBuffer=null,this.dirty=!1}},{key:"kill",value:function(){this.killed=!0,this.rafId!==null&&cancelAnimationFrame(this.rafId),this.rafId=null;var n=this.options.gl;this.fence&&n.deleteSync(this.fence),this.fence=null,this.readBuffer&&n.deleteBuffer(this.readBuffer),this.readBuffer=null}}])})(),Js=(function(){function e(t,n){K(this,e),this.key=t,this.size=n}return Z(e,null,[{key:"compare",value:function(n,r){return n.size>r.size?-1:n.size<r.size||n.key>r.key?1:-1}}])})(),Ir=(function(){function e(){K(this,e),R(this,"width",0),R(this,"height",0),R(this,"cellSize",0),R(this,"columns",0),R(this,"rows",0),R(this,"cells",{})}return Z(e,[{key:"resizeAndClear",value:function(n,r){this.width=n.width,this.height=n.height,this.cellSize=r,this.columns=Math.ceil(n.width/r),this.rows=Math.ceil(n.height/r),this.cells={}}},{key:"getIndex",value:function(n){var r=Math.floor(n.x/this.cellSize),a=Math.floor(n.y/this.cellSize);return a*this.columns+r}},{key:"add",value:function(n,r,a){var i=new Js(n,r),o=this.getIndex(a),s=this.cells[o];s||(s=[],this.cells[o]=s),s.push(i)}},{key:"organize",value:function(){for(var n in this.cells){var r=this.cells[n];r.sort(Js.compare)}}},{key:"getLabelsToDisplay",value:function(n,r,a){var i=this.cellSize*this.cellSize,o=i/n/n,s=o*r/i,l=Math.ceil(s),u=[];if(a)for(var c=Math.max(0,Math.floor(a.x1/this.cellSize)),d=Math.min(this.columns-1,Math.floor(a.x2/this.cellSize)),f=Math.max(0,Math.floor(a.y1/this.cellSize)),h=Math.min(this.rows-1,Math.floor(a.y2/this.cellSize)),g=f;g<=h;g++)for(var m=c;m<=d;m++){var v=g*this.columns+m,p=this.cells[v];if(p)for(var b=0;b<Math.min(l,p.length);b++)u.push(p[b].key)}else for(var y in this.cells)for(var _=this.cells[y],x=0;x<Math.min(l,_.length);x++)u.push(_[x].key);return u}}])})();function Tg(e){var t=e.graph,n=e.hoveredNode,r=e.highlightedNodes,a=e.displayedNodeLabels,i=new Set,o=new Set(r);n&&o.add(n);var s=G(o),l;try{for(s.s();!(l=s.n()).done;){var u=l.value;t.hasNode(u)&&t.forEachEdge(u,function(h){i.add(h)})}}catch(h){s.e(h)}finally{s.f()}var c=G(a),d;try{var f=function(){var g=d.value;if(o.has(g))return 1;t.forEachEdge(g,function(m,v,p,b){var y=p===g?b:p;a.has(y)&&i.add(m)})};for(c.s();!(d=c.n()).done;)f()}catch(h){c.e(h)}finally{c.f()}return Array.from(i)}var qt=150,Xt=50,Eg={both:0,node:1,label:2},Sg={over:0,above:1,below:2,auto:3},Qs=12,Rg=3,Ag=(function(){function e(t){K(this,e),R(this,"labelGrid",new Ir),R(this,"edgeAnchorGrid",new Ir),R(this,"displayedNodeLabels",new Set),R(this,"displayedEdgeLabels",new Set),R(this,"edgeLabelCandidates",[]),R(this,"renderedNodeLabels",new Set),R(this,"labelSizeCache",new Map),R(this,"framePassPoints",new Float32Array(0)),R(this,"labelsDirty",!0),this.internals=t}return Z(e,[{key:"resetFrame",value:function(){this.displayedNodeLabels=new Set,this.renderedNodeLabels=new Set,this.labelSizeCache.clear()}},{key:"clearEdgeLabels",value:function(){this.displayedEdgeLabels=new Set}},{key:"resetLabelGrid",value:function(){this.labelGrid=new Ir,this.edgeAnchorGrid=new Ir}},{key:"processWebGLLabels",value:function(n){for(var r,a=this.internals,i=a.labelProgram,o=a.primitives,s=a.nodeDataCache,l=(o==null||(r=o.nodes)===null||r===void 0||(r=r.label)===null||r===void 0||(r=r.font)===null||r===void 0?void 0:r.family)||"sans-serif",u=new Map,c=0,d=n.length;c<d;c++){var f=n[c],h=s[f];if(!(h.visibility==="hidden"||!h.label)){var g=h.labelFont||l,m=u.get(g);m?m.push(h.label):u.set(g,[h.label])}}var v=G(u),p;try{for(v.s();!(p=v.n()).done;){var b=Q(p.value,2),y=b[0],_=b[1],x=pr(y),T=x.family,E=x.weight,S=x.style,w=i.registerFont(T,E,S);i.ensureGlyphsReady(_,w)}}catch(A){v.e(A)}finally{v.f()}}},{key:"measureNodeLabel",value:function(n){var r,a;if(!n.label)return{width:0,height:0,textHeight:0};var i=this.internals,o=i.labelProgram,s=i.primitives,l=(r=n.labelSize)!==null&&r!==void 0?r:14,u=n.labelFont||(s==null||(a=s.nodes)===null||a===void 0||(a=a.label)===null||a===void 0||(a=a.font)===null||a===void 0?void 0:a.family)||"sans-serif",c="".concat(n.label,"|").concat(l,"|").concat(u);if(this.labelSizeCache.has(c))return this.labelSizeCache.get(c);var d=pr(u),f=d.family,h=d.weight,g=d.style,m=o.registerFont(f,h,g),v=o.measureLabel(n.label,l,m);return this.labelSizeCache.set(c,v),v}},{key:"nodeLabelBox",value:function(n,r){var a,i,o,s,l;if(n.visibility==="hidden"||n.labelVisibility==="hidden")return null;var u=this.measureNodeLabel(n),c=u.width,d=u.height,f=u.textHeight;if(!c)return null;var h=(a=(i=this.internals.primitives)===null||i===void 0||(i=i.nodes)===null||i===void 0||(i=i.label)===null||i===void 0?void 0:i.margin)!==null&&a!==void 0?a:bt,g=r+h,m=n.labelBackgroundColor?(o=n.labelBackgroundPadding)!==null&&o!==void 0?o:wr:0,v=c/2,p=d/2,b=v+m,y=p+m,_=f/2,x=0,T=0;switch((s=n.labelPosition)!==null&&s!==void 0?s:"right"){case"left":x=-(g+v);break;case"above":T=-(g+_);break;case"below":T=g+_;break;case"over":break;default:x=g+v}var E=(l=n.labelAngle)!==null&&l!==void 0?l:0,S=er({x,y:T},E),w=S.x,A=S.y,C=Math.abs(Math.cos(E)),D=Math.abs(Math.sin(E)),F=C*b+D*y,k=D*b+C*y;return{minX:w-F,maxX:w+F,minY:A-k,maxY:A+k}}},{key:"computeDisplayedNodeLabels",value:function(){var n=this.internals.getCameraState(),r=this.internals.getDimensions(),a=r.width,i=r.height,o=this.internals.viewportToFramedGraph({x:-qt,y:-Xt}),s=this.internals.viewportToFramedGraph({x:a+qt,y:-Xt}),l=this.internals.viewportToFramedGraph({x:-qt,y:i+Xt}),u=this.internals.viewportToFramedGraph({x:a+qt,y:i+Xt}),c=Math.min(o.x,s.x,l.x,u.x),d=Math.max(o.x,s.x,l.x,u.x),f=Math.min(o.y,s.y,l.y,u.y),h=Math.max(o.y,s.y,l.y,u.y),g=Ke({x:.5,y:.5,ratio:1,angle:0},{width:a,height:i},this.internals.getGraphDimensions(),this.internals.getStagePadding()),m=function(I){var M=ht(g,I);return{x:(1+M.x)*a/2,y:(1-M.y)*i/2}},v=m({x:c,y:f}),p=m({x:d,y:f}),b=m({x:c,y:h}),y=m({x:d,y:h}),_={x1:Math.min(v.x,p.x,b.x,y.x),y1:Math.min(v.y,p.y,b.y,y.y),x2:Math.max(v.x,p.x,b.x,y.x),y2:Math.max(v.y,p.y,b.y,y.y)},x=this.internals,T=x.settings,E=x.nodeDataCache,S=x.nodesWithForcedLabels,w=this.labelGrid.getLabelsToDisplay(n.ratio,T.labelDensity,_);Aa(w,S);for(var A=0,C=w.length;A<C;A++){var D=w[A],F=E[D];if(!this.displayedNodeLabels.has(D)&&!(F.visibility==="hidden"||F.labelVisibility==="hidden")&&F.label&&!(F.x<c||F.x>d||F.y<f||F.y>h)){var k=this.internals.framedGraphToViewport(F),L=k.x,z=k.y,N=this.internals.scaleSize(F.size);!Qe(F)&&N<T.labelRenderedSizeThreshold||L<-qt-N||L>a+qt+N||z<-Xt-N||z>i+Xt+N||this.displayedNodeLabels.add(D)}}}},{key:"buildFramePassPoints",value:function(){var n=this.internals,r=n.nodeDataCache,a=n.nodeDataTexture;if(!a)return{data:this.framePassPoints,count:0};var i=this.displayedNodeLabels.size*3;this.framePassPoints.length<i&&(this.framePassPoints=new Float32Array(i));var o=this.framePassPoints,s=0,l=G(this.displayedNodeLabels),u;try{for(l.s();!(u=l.n()).done;){var c,d,f=u.value,h=r[f];if(h){var g=a.getIndex(f);g<0||(o[s*3]=g,o[s*3+1]=(c=mt[h.labelPosition||Je.labelPosition])!==null&&c!==void 0?c:0,o[s*3+2]=(d=h.labelAngle)!==null&&d!==void 0?d:0,s++)}}}catch(m){l.e(m)}finally{l.f()}return{data:o,count:s}}},{key:"renderWebGLLabels",value:function(n,r){var a=this.internals,i=a.nodeDataCache,o=a.labelProgram,s=a.primitives,l=a.nodeDataTexture,u=[],c=G(this.displayedNodeLabels),d;try{for(c.s();!(d=c.n()).done;){var f=d.value;if(!this.renderedNodeLabels.has(f)){var h=i[f];r&&h.labelDepth!==r||(this.renderedNodeLabels.add(f),u.push(f))}}}catch(ye){c.e(ye)}finally{c.f()}if(u.length!==0){if(this.labelsDirty){for(var g,m,v,p=0,b=0,y=u.length;b<y;b++){var _=i[u[b]];p+=_.label.length}o.reallocate(p);for(var x=14,T=(g=s==null||(m=s.nodes)===null||m===void 0||(m=m.label)===null||m===void 0?void 0:m.margin)!==null&&g!==void 0?g:bt,E=Je.labelPosition,S=(s==null||(v=s.nodes)===null||v===void 0||(v=v.label)===null||v===void 0||(v=v.font)===null||v===void 0?void 0:v.family)||"sans-serif",w=new Map,A=0,C=0,D=u.length;C<D;C++){var F,k,L,z,N=u[C],P=i[N],I=P.labelFont||S,M=w.get(I);if(M===void 0){var j=pr(I),U=j.family,Y=j.weight,q=j.style;M=o.registerFont(U,Y,q),w.set(I,M)}var X={text:P.label,x:P.x,y:P.y,size:(F=P.labelSize)!==null&&F!==void 0?F:x,color:P.labelColor,nodeSize:P.size,margin:T,position:(k=P.labelPosition)!==null&&k!==void 0?k:E,hidden:!1,forceLabel:Qe(P),type:"default",zIndex:(L=P.zIndex)!==null&&L!==void 0?L:0,parentType:"node",parentKey:N,fontKey:M,labelAngle:(z=P.labelAngle)!==null&&z!==void 0?z:0,nodeIndex:l.getIndex(N)},Le=o.processLabel(N,A,X);A+=Le}o.invalidateBuffers()}o.render(n)}}},{key:"renderBackdrops",value:function(n,r){var a=this.internals,i=a.backdropProgram,o=a.nodeDataCache,s=a.nodesWithBackdrop,l=a.attachmentManager,u=a.pixelRatio,c=a.nodeDataTexture,d=[],f=G(s),h;try{for(f.s();!(h=f.n()).done;){var g,m=h.value,v=o[m];if(!(!v||v.visibility==="hidden")&&!(r&&v.depth!==r)){var p=(g=c?.getIndex(m))!==null&&g!==void 0?g:-1;p<0||d.push({key:m,nodeIndex:p})}}}catch(ct){f.e(ct)}finally{f.f()}if(d.length!==0){i.reallocate(d.length);for(var b=0;b<d.length;b++){var y,_,x,T,E,S,w,A,C=d[b],D=C.key,F=C.nodeIndex,k=o[D],L=this.displayedNodeLabels.has(D),z=L?this.measureNodeLabel(k):{width:0,height:0,textHeight:0},N=z.textHeight,P=z.width,I=z.height,M=0,j=0;if(L&&k.labelAttachment&&l){var U=l.getEntry(D,k.labelAttachment);if(U){var Y=k.labelAttachmentPlacement||"below";if(Y==="below"||Y==="above"){var q=U.height/u;I+=q+at,P=Math.max(P,U.width/u),j=Y==="below"?(q+at)/2:-(q+at)/2}else{var X=U.width/u;P+=X+at,I=Math.max(I,U.height/u),M=Y==="right"?(X+at)/2:-(X+at)/2}}}var Le=k.backdropColor?Ze(k.backdropColor):[255,255,255,255],ye=k.backdropShadowColor?Ze(k.backdropShadowColor):[0,0,0,128],Ve=Le.map(function(ct){return ct/255}),wt=ye.map(function(ct){return ct/255}),Qt=(y=k.backdropShadowBlur)!==null&&y!==void 0?y:12,en=(_=k.backdropPadding)!==null&&_!==void 0?_:6,tn=k.backdropBorderColor?Ze(k.backdropBorderColor):[0,0,0,0],dc=tn.map(function(ct){return ct/255}),hc=(x=k.backdropBorderWidth)!==null&&x!==void 0?x:0,fc=(T=k.backdropCornerRadius)!==null&&T!==void 0?T:0,Fi=(E=k.backdropLabelPadding)!==null&&E!==void 0?E:-1,gc=Fi<0?en:Fi,pc=(S=Eg[(w=k.backdropArea)!==null&&w!==void 0?w:"both"])!==null&&S!==void 0?S:0,vc={key:D,nodeIndex:F,label:k.label,labelWidth:P,labelHeight:I,textHeight:N,type:"default",position:k.labelPosition||Je.labelPosition,labelAngle:(A=k.labelAngle)!==null&&A!==void 0?A:0,backdropColor:Ve,backdropShadowColor:wt,backdropShadowBlur:Qt,backdropPadding:en,backdropBorderColor:dc,backdropBorderWidth:hc,backdropCornerRadius:fc,backdropLabelPadding:gc,backdropArea:pc,labelBoxOffset:[M,j]};i.processBackdrop(b,vc)}i.invalidateBuffers(),i.render(n)}}},{key:"renderLabelBackgrounds",value:function(n,r){var a=this.internals,i=a.labelBackgroundProgram,o=a.nodeDataCache,s=a.nodeDataTexture,l=ge.nodeLabel.writesPickingThisFrame(this.internals),u=[],c=G(this.displayedNodeLabels),d;try{for(c.s();!(d=c.n()).done;){var f=d.value,h=o[f];!h||h.visibility==="hidden"||r&&h.labelDepth!==r||!l&&!h.labelBackgroundColor||u.push(f)}}catch(F){c.e(F)}finally{c.f()}if(u.length!==0){i.reallocate(u.length);for(var g=0;g<u.length;g++){var m,v,p,b,y=u[g],_=o[y],x=(m=s?.getIndex(y))!==null&&m!==void 0?m:-1;if(!(x<0)){var T=this.measureNodeLabel(_),E=T.width,S=T.height,w=T.textHeight,A=Tt(this.internals.pickingState,"nodeLabel",y)||Tt(this.internals.pickingState,"node",y),C=_.labelBackgroundColor?xe(_.labelBackgroundColor):xe("transparent"),D={nodeIndex:x,id:ft(A),color:C,labelWidth:E,labelHeight:S,textHeight:w,positionMode:(v=mt[_.labelPosition||Je.labelPosition])!==null&&v!==void 0?v:0,labelAngle:(p=_.labelAngle)!==null&&p!==void 0?p:0,padding:(b=_.labelBackgroundPadding)!==null&&b!==void 0?b:wr};i.processLabelBackground(g,D)}}i.invalidateBuffers(),i.render(n)}}},{key:"cacheAttachments",value:function(n){var r=this.internals,a=r.attachmentManager,i=r.pixelRatio,o=r.nodeDataCache,s=r.nodesWithBackdrop,l=r.graph;if(a){var u=G(s),c;try{for(u.s();!(c=u.n()).done;){var d=c.value;if(this.displayedNodeLabels.has(d)){var f=o[d];if(!(!f||f.visibility==="hidden")&&!(n&&f.depth!==n)&&f.labelAttachment){var h=l.getNodeAttributes(d),g=this.measureNodeLabel(f),m=g.width,v=g.height,p={node:d,attributes:h,pixelRatio:i,labelWidth:m,labelHeight:v};a.renderAttachment(d,f.labelAttachment,p)}}}}catch(b){u.e(b)}finally{u.f()}a.regenerateAtlas()}}},{key:"renderAttachments",value:function(n,r){var a=this.internals,i=a.attachmentManager,o=a.attachmentProgram,s=a.nodeDataCache,l=a.nodesWithBackdrop,u=a.pixelRatio,c=a.nodeDataTexture;if(!(!i||!o)){var d=0;o.reallocateAttachments(l.size);var f=G(l),h;try{for(f.s();!(h=f.n()).done;){var g,m,v,p,b=h.value;if(this.displayedNodeLabels.has(b)){var y=s[b];if(!(!y||y.visibility==="hidden")&&!(r&&y.labelDepth!==r)&&y.labelAttachment){var _=i.getEntry(b,y.labelAttachment);if(_){var x=(g=c?.getIndex(b))!==null&&g!==void 0?g:-1;if(!(x<0)){var T=this.measureNodeLabel(y),E=T.width,S=T.height,w=T.textHeight,A=(m=rs[y.labelAttachmentPlacement||"below"])!==null&&m!==void 0?m:0;o.processAttachment(d,{nodeIndex:x,atlasX:_.x,atlasY:_.y,atlasW:_.width,atlasH:_.height,attachWidth:_.width/u,attachHeight:_.height/u,positionMode:(v=mt[y.labelPosition||Je.labelPosition])!==null&&v!==void 0?v:0,attachmentPlacement:A,labelWidth:E,labelHeight:S,textHeight:w,labelAngle:(p=y.labelAngle)!==null&&p!==void 0?p:0}),d++}}}}}}catch(C){f.e(C)}finally{f.f()}d!==0&&(o.reallocateAttachments(d),i.bindTexture(xn),o.invalidateBuffers(),o.render(n))}}},{key:"computeDisplayedEdgeLabels",value:function(){var n=this.internals,r=n.graph,a=n.stateManager,i=n.settings,o=n.edgesWithForcedLabels,s=a.getHighlightedNodes(),l=i.edgeLabelAnchors==="allNodes"?new Set(this.edgeAnchorGrid.getLabelsToDisplay(this.internals.getCameraState().ratio,i.labelDensity)):this.displayedNodeLabels,u=a.hovered,c=Tg({graph:r,hoveredNode:u?.kind==="node"?u.key:null,displayedNodeLabels:l,highlightedNodes:s});Aa(c,o),this.edgeLabelCandidates=c,this.displayedEdgeLabels=new Set}},{key:"filterEdgeLabelsForDepth",value:function(n){for(var r=this.internals,a=r.graph,i=r.nodeDataCache,o=r.edgeDataCache,s=[],l=new Set,u=0,c=this.edgeLabelCandidates.length;u<c;u++){var d=this.edgeLabelCandidates[u];if(!l.has(d)){l.add(d);var f=a.extremities(d),h=i[f[0]],g=i[f[1]],m=o[d];!m||!h||!g||m.visibility==="hidden"||m.labelVisibility==="hidden"||h.visibility==="hidden"||g.visibility==="hidden"||n&&m.labelDepth!==n||m.label&&s.push(d)}}return s}},{key:"renderEdgeLabels",value:function(n,r){var a=this.internals,i=a.graph,o=a.nodeDataCache,s=a.edgeDataCache,l=a.primitives,u=a.edgeLabelProgram,c=a.nodeDataTexture,d=a.edgeDataTexture;if(u){var f=this.filterEdgeLabelsForDepth(r),h=G(f),g;try{for(h.s();!(g=h.n()).done;){var m=g.value;this.displayedEdgeLabels.add(m)}}catch(X){h.e(X)}finally{h.f()}if(f.length!==0){if(this.labelsDirty){var v,p,b=0,y=G(f),_;try{for(y.s();!(_=y.n()).done;){var x=_.value;b+=s[x].label.length}}catch(X){y.e(X)}finally{y.f()}u.reallocate(b);var T=(v=l==null||(p=l.edges)===null||p===void 0||(p=p.label)===null||p===void 0?void 0:p.margin)!==null&&v!==void 0?v:5,E="over",S=0,w=G(f),A;try{for(w.s();!(A=w.n()).done;){var C,D,F=A.value,k=i.extremities(F),L=k[0],z=k[1],N=o[L],P=o[z],I=s[F],M=c.getIndex(L),j=c.getIndex(z),U=d.getIndex(F),Y={text:I.label,x:(N.x+P.x)/2,y:(N.y+P.y)/2,size:Qs,color:I.labelColor,nodeSize:0,nodeIndex:-1,margin:T,position:(C=I.labelPosition)!==null&&C!==void 0?C:E,hidden:!1,forceLabel:Qe(I),type:"default",zIndex:(D=I.zIndex)!==null&&D!==void 0?D:0,parentType:"edge",parentKey:F,fontKey:"",labelAngle:0,sourceX:N.x,sourceY:N.y,targetX:P.x,targetY:P.y,sourceSize:N.size,targetSize:P.size,sourceShape:N.shape||"circle",targetShape:P.shape||"circle",edgeSize:I.size,offset:0,edgeAttributes:I,sourceNodeIndex:M,targetNodeIndex:j,edgeIndex:U},q=u.processEdgeLabel(F,S,Y);S+=q}}catch(X){w.e(X)}finally{w.f()}u.invalidateBuffers()}u.render(n)}}}},{key:"renderEdgeLabelBackgrounds",value:function(n,r){var a,i,o=this.internals,s=o.edgeLabelBackgroundProgram,l=o.edgeLabelProgram,u=o.edgeDataCache,c=o.primitives,d=o.edgeDataTexture;if(!(!d||!l||!s)){var f=ge.edgeLabel.writesPickingThisFrame(this.internals),h=(a=c==null||(i=c.edges)===null||i===void 0||(i=i.label)===null||i===void 0?void 0:i.margin)!==null&&a!==void 0?a:5,g="over",m=this.filterEdgeLabelsForDepth(r),v=[],p=G(m),b;try{for(p.s();!(b=p.n()).done;){var y=b.value;!f&&!u[y].labelBackgroundColor||v.push(y)}}catch(N){p.e(N)}finally{p.f()}if(v.length!==0){s.reallocate(v.length);for(var _=0;_<v.length;_++){var x,T,E,S=v[_],w=u[S],A=w.label,C=l.measureLabelAtlasWidth(A),D=(x=w.labelPosition)!==null&&x!==void 0?x:g,F=typeof D=="string"&&(T=Sg[D])!==null&&T!==void 0?T:0,k=Tt(this.internals.pickingState,"edgeLabel",S)||Tt(this.internals.pickingState,"edge",S),L=w.labelBackgroundColor?xe(w.labelBackgroundColor):xe("transparent"),z={edgeIndex:d.getIndex(S),baseFontSize:Qs,totalTextWidth:C,positionMode:F,margin:h,padding:(E=w.labelBackgroundPadding)!==null&&E!==void 0?E:Rg,color:L,id:ft(k),edgeAttributes:w};s.processEdgeLabelBackground(_,S,z)}s.invalidateBuffers(),s.render(n)}}}}])})(),wg=(function(){function e(t,n,r,a){K(this,e),R(this,"nodeStates",new Map),R(this,"edgeStates",new Map),R(this,"hovered",null),R(this,"dirtyNodes",new Set),R(this,"dirtyEdges",new Set),R(this,"graphStateChanged",!1),R(this,"graphStateFlagsDirty",!1),this.scheduleRefresh=t,this.customNodeStateDefaults=n,this.customEdgeStateDefaults=r,this.customGraphStateDefaults=a,this.graphState=ya(a)}return Z(e,[{key:"getNodeState",value:function(n){n=""+n;var r=this.nodeStates.get(n);return r||(r=Eo(this.customNodeStateDefaults),this.nodeStates.set(n,r)),r}},{key:"getEdgeState",value:function(n){n=""+n;var r=this.edgeStates.get(n);return r||(r=So(this.customEdgeStateDefaults),this.edgeStates.set(n,r)),r}},{key:"getGraphState",value:function(){return this.flushGraphStateFlags(),this.graphState}},{key:"getHighlightedNodes",value:function(){var n=new Set,r=G(this.nodeStates),a;try{for(r.s();!(a=r.n()).done;){var i=Q(a.value,2),o=i[0],s=i[1];s.isHighlighted&&n.add(o)}}catch(l){r.e(l)}finally{r.f()}return n}},{key:"setNodeState",value:function(n,r){n=""+n;var a=this.getNodeState(n);if(Gt(a,r)){var i=B(B({},a),r);this.nodeStates.set(n,i),this.dirtyNodes.add(n),this.updateHoveredNodeTracking(n,a,i),this.graphStateFlagsDirty=!0,this.scheduleRefresh()}}},{key:"setEdgeState",value:function(n,r){n=""+n;var a=this.getEdgeState(n);if(Gt(a,r)){var i=B(B({},a),r);this.edgeStates.set(n,i),this.dirtyEdges.add(n),this.updateHoveredEdgeTracking(n,a,i),this.graphStateFlagsDirty=!0,this.scheduleRefresh()}}},{key:"setGraphState",value:function(n){if(Gt(this.graphState,n)){var r=B(B({},this.graphState),n);r.isIdle=!r.isPanning&&!r.isZooming&&!r.isDragging,this.graphState=r,this.graphStateChanged=!0,this.scheduleRefresh()}}},{key:"setNodesState",value:function(n,r){var a=!1,i=G(n),o;try{for(i.s();!(o=i.n()).done;){var s=o.value;s=""+s;var l=this.getNodeState(s);if(Gt(l,r)){var u=B(B({},l),r);this.nodeStates.set(s,u),this.dirtyNodes.add(s),this.updateHoveredNodeTracking(s,l,u),a=!0}}}catch(c){i.e(c)}finally{i.f()}a&&(this.graphStateFlagsDirty=!0,this.scheduleRefresh())}},{key:"setEdgesState",value:function(n,r){var a=!1,i=G(n),o;try{for(i.s();!(o=i.n()).done;){var s=o.value;s=""+s;var l=this.getEdgeState(s);if(Gt(l,r)){var u=B(B({},l),r);this.edgeStates.set(s,u),this.dirtyEdges.add(s),this.updateHoveredEdgeTracking(s,l,u),a=!0}}}catch(c){i.e(c)}finally{i.f()}a&&(this.graphStateFlagsDirty=!0,this.scheduleRefresh())}},{key:"removeNode",value:function(n){this.nodeStates.delete(n),this.dirtyNodes.delete(n),this.clearHoveredFor("node",n)}},{key:"removeEdge",value:function(n){this.edgeStates.delete(n),this.dirtyEdges.delete(n),this.clearHoveredFor("edge",n)}},{key:"pruneNodes",value:function(n){var r=G(this.nodeStates.keys()),a;try{for(r.s();!(a=r.n()).done;){var i=a.value;n(i)||this.removeNode(i)}}catch(o){r.e(o)}finally{r.f()}}},{key:"pruneEdges",value:function(n){var r=G(this.edgeStates.keys()),a;try{for(r.s();!(a=r.n()).done;){var i=a.value;n(i)||this.removeEdge(i)}}catch(o){r.e(o)}finally{r.f()}}},{key:"clearNodes",value:function(){this.nodeStates.clear(),this.dirtyNodes.clear(),this.clearHoveredForKinds(Qa("node"))}},{key:"clearEdges",value:function(){this.edgeStates.clear(),this.dirtyEdges.clear(),this.clearHoveredForKinds(Qa("edge"))}},{key:"resetGraphState",value:function(){this.graphState=ya(this.customGraphStateDefaults),this.graphStateChanged=!1,this.graphStateFlagsDirty=!1}},{key:"clearDirtyTracking",value:function(){this.dirtyNodes.clear(),this.dirtyEdges.clear(),this.graphStateChanged=!1}},{key:"setHovered",value:function(n){this.hovered=n}},{key:"clearHoveredFor",value:function(n,r){this.hovered&&this.hovered.key===r&&Qa(n).includes(this.hovered.kind)&&(this.hovered=null)}},{key:"clearHoveredForKinds",value:function(n){this.hovered&&n.includes(this.hovered.kind)&&(this.hovered=null)}},{key:"updateGraphStateFromNodes",value:function(){var n,r=!1,a=!1,i=!1,o=G(this.nodeStates),s;try{for(o.s();!(s=o.n()).done;){var l=Q(s.value,2),u=l[1];if(u.isHovered&&(r=!0),u.isHighlighted&&(a=!0),u.isDragged&&(i=!0),r&&a&&i)break}}catch(d){o.e(d)}finally{o.f()}!r&&((n=this.hovered)===null||n===void 0?void 0:n.kind)==="edge"&&(r=!0);var c=!this.graphState.isPanning&&!this.graphState.isZooming&&!i;(this.graphState.hasHovered!==r||this.graphState.hasHighlighted!==a||this.graphState.isDragging!==i||this.graphState.isIdle!==c)&&(this.graphStateChanged=!0),this.graphState=B(B({},this.graphState),{},{hasHovered:r,hasHighlighted:a,isDragging:i,isIdle:c})}},{key:"updateGraphStateFromEdges",value:function(){var n,r=((n=this.hovered)===null||n===void 0?void 0:n.kind)==="node";if(!r){var a=G(this.edgeStates),i;try{for(a.s();!(i=a.n()).done;){var o=Q(i.value,2),s=o[1];if(s.isHovered){r=!0;break}}}catch(l){a.e(l)}finally{a.f()}}this.graphState.hasHovered!==r&&(this.graphStateChanged=!0),this.graphState=B(B({},this.graphState),{},{hasHovered:r})}},{key:"flushGraphStateFlags",value:function(){this.graphStateFlagsDirty&&(this.updateGraphStateFromNodes(),this.updateGraphStateFromEdges(),this.graphStateFlagsDirty=!1)}},{key:"updateHoveredNodeTracking",value:function(n,r,a){var i;if(r.isHovered!==a.isHovered)if(a.isHovered){var o;if(((o=this.hovered)===null||o===void 0?void 0:o.kind)==="node"&&this.hovered.key!==n){var s=this.hovered.key,l=this.getNodeState(s);this.nodeStates.set(s,B(B({},l),{},{isHovered:!1})),this.dirtyNodes.add(s)}this.hovered={kind:"node",key:n}}else((i=this.hovered)===null||i===void 0?void 0:i.kind)==="node"&&this.hovered.key===n&&(this.hovered=null)}},{key:"updateHoveredEdgeTracking",value:function(n,r,a){var i;if(r.isHovered!==a.isHovered)if(a.isHovered){var o;if(((o=this.hovered)===null||o===void 0?void 0:o.kind)==="edge"&&this.hovered.key!==n){var s=this.hovered.key,l=this.getEdgeState(s);this.edgeStates.set(s,B(B({},l),{},{isHovered:!1})),this.dirtyEdges.add(s)}this.hovered={kind:"edge",key:n}}else((i=this.hovered)===null||i===void 0?void 0:i.kind)==="edge"&&this.hovered.key===n&&(this.hovered=null)}}])})(),el=1,tl=2,nl=3,rl=4,Dg=(function(e){function t(n,r){var a,i,o,s,l,u=arguments.length>2&&arguments[2]!==void 0?arguments[2]:{};K(this,t),l=ae(this,t),R(l,"nodeReducer",null),R(l,"edgeReducer",null),R(l,"stageCanvas",null),R(l,"mouseLayer",null),R(l,"gestureHint",null),R(l,"extraElements",{}),R(l,"webGLContext",null),R(l,"pickingFrameBuffer",null),R(l,"pickingTexture",null),R(l,"pickingDepthBuffer",null),R(l,"activeListeners",{}),R(l,"nodeVariableEntries",[]),R(l,"edgeVariableEntries",[]),R(l,"edgePathsByName",new Map),R(l,"nodeProgramIndex",{}),R(l,"edgeProgramIndex",{}),R(l,"edgeTextureIndexCache",{}),R(l,"nodeGraphCoords",{}),R(l,"nodeExtent",{x:[0,1],y:[0,1]}),R(l,"matrix",_e()),R(l,"invMatrix",_e()),R(l,"correctionRatio",1),R(l,"frameId",0),R(l,"customBBox",null),R(l,"gpuTimerExt",void 0),R(l,"activeGpuTimerQuery",null),R(l,"pendingGpuTimerQueries",[]),R(l,"normalizationFunction",hr({x:[0,1],y:[0,1]})),R(l,"graphToViewportRatio",1),R(l,"pickingState",ng()),R(l,"prevNodeVisibilities",{}),R(l,"width",0),R(l,"height",0),R(l,"autoRescaleFrozen",!1),R(l,"stylesDeclaration",null),R(l,"resolvedStageStyle",{}),R(l,"renderFrame",null),R(l,"contextLost",!1),R(l,"pendingProcess","full"),R(l,"needToRefreshState",!1),R(l,"checkEdgesEventsFrame",null),R(l,"edgeStyleAnalysis",{dependency:"static",xAttribute:null,yAttribute:null}),R(l,"depthLayers",$(ir)),R(l,"customLayerPrograms",new Map),R(l,"nodeShapeSlug",null),R(l,"sdfAtlas",null),R(l,"depthRanges",{nodes:{},edges:{}}),R(l,"nodeBaseDepth",{}),R(l,"edgeBaseDepth",{});var c=u.primitives,d=u.styles,f=u.settings,h=f===void 0?{}:f,g=u.nodeReducer,m=u.edgeReducer,v=u.customNodeState,p=u.customEdgeState,b=u.customGraphState;l.stateManager=new wg(function(){return l.scheduleStateRefresh()},v,p,b);var y=c??wo;l.stylesDeclaration=d?{nodes:(a=d.nodes)!==null&&a!==void 0?a:lr.nodes,edges:(i=d.edges)!==null&&i!==void 0?i:lr.edges,stage:d.stage}:lr,l.nodeReducer=g??null,l.edgeReducer=m??null;var _=_a(l.stylesDeclaration.nodes);l.nodeReducer&&(_.dependency="graph-state"),l.edgeStyleAnalysis=_a(l.stylesDeclaration.edges),l.edgeReducer&&(l.edgeStyleAnalysis.dependency="graph-state"),l.stylesDeclaration.stage&&(l.resolvedStageStyle=Ra(l.stylesDeclaration.stage,l.stateManager.graphState));var x=Io(h);if(gr(x),x.enableNodeDrag){var T=_.xAttribute,E=_.yAttribute;if((!T||!E)&&!x.dragPositionToAttributes)throw new Error('Sigma: `enableNodeDrag` is true but position attribute names could not be inferred from styles. Either use attribute bindings for x/y in your node styles (e.g. `x: { attribute: "x" }`), or provide a `dragPositionToAttributes` setting.')}if(Po(n),!(r instanceof HTMLElement))throw new Error("Sigma: container should be an html element.");l.container=r,l.edgeGroups=new tg(n,function(P,I){for(var M=0;M<P.length;M++){var j=l.stateManager.getEdgeState(P[M]);j.parallelIndex=M,j.parallelCount=I}});var S=new eg(n,l.viewportToGraph.bind(l),l.setNodesState.bind(l),function(P,I){return l.emit(P,I)});if(l.depthLayers=(o=y.depthLayers)!==null&&o!==void 0?o:$(ir),!(d!=null&&d.nodes)&&!rr.every(function(P){return l.depthLayers.includes(P)}))throw new Error("Sigma: depthLayers must include ".concat(rr.join(", ")," for the built-in node styles."));if(!(d!=null&&d.edges)&&!ar.every(function(P){return l.depthLayers.includes(P)}))throw new Error("Sigma: depthLayers must include ".concat(ar.join(", ")," for the built-in edge styles."));l.itemBuckets={nodes:new Ya(l.depthLayers),edges:new Ya(l.depthLayers)},l.initWebGLContext(),l.mouseLayer=Wt("div",{position:"absolute",touchAction:"none",userSelect:"none"},{class:"sigma-mouse"}),l.container.appendChild(l.mouseLayer),l.resolvedStageStyle.background&&(l.container.style.backgroundColor=l.resolvedStageStyle.background),l.resolvedStageStyle.cursor&&(l.container.style.cursor=l.resolvedStageStyle.cursor);var w=new Bs(l.webGLContext),A=new Ka(l.webGLContext,{channels:1}),C=new Ws(l.webGLContext),D=new Ka(l.webGLContext,{channels:4}),F=l.webGLContext,k=new xg({gl:F,getFrameBuffer:function(){return l.pickingFrameBuffer},getPixelRatio:function(){return l.internals.pixelRatio},getDownSizingRatio:function(){return l.internals.settings.pickingDownSizingRatio},isRenderPending:function(){return l.renderFrame!==null},onIndex:function(I,M){var j;return sl(l.internals,(j=l.pickingState.lookup[I])!==null&&j!==void 0?j:null,M)}}),L=l.initPrograms(y,x),z=y==null||(s=y.nodes)===null||s===void 0?void 0:s.labelAttachments,N=null;return z&&Object.keys(z).length>0&&(N=new Gs(F,z,function(){return l.scheduleRender()})),l.internals=B(B({nodeDataCache:{},edgeDataCache:{},nodesWithForcedLabels:new Set,nodesWithBackdrop:new Set,edgesWithForcedLabels:new Set,settings:x,primitives:y,pixelRatio:gn(),graph:n,stateManager:l.stateManager,dragManager:S,hoverResolver:k,nodeStyleAnalysis:_,pickingState:l.pickingState},L),{},{attachmentManager:N,nodeDataTexture:w,nodeFrameTexture:A,edgeDataTexture:C,edgeFrameTexture:D,getDimensions:function(){return l.getDimensions()},getGraphDimensions:function(){return l.getGraphDimensions()},getStagePadding:function(){return l.getStagePadding()},getCameraState:function(){return l.camera.getState()},getHitAtPosition:function(I){return l.getHitAtPosition(I)},setNodeState:function(I,M){return l.setNodeState(I,M)},setEdgeState:function(I,M){return l.setEdgeState(I,M)},updateContainerCursor:function(){return l.updateContainerCursor()},scheduleRefresh:function(){return l.scheduleRefresh()},viewportToFramedGraph:function(I){return l.viewportToFramedGraph(I)},viewportToGraph:function(I){return l.viewportToGraph(I)},framedGraphToViewport:function(I){return l.framedGraphToViewport(I)},scaleSize:function(I){return l.scaleSize(I)},emit:function(I,M){return l.emit(I,M)}}),l.labelRenderer=new Ag(l.internals),l.resize(),l.initializeWebGLLabels(),l.camera=new Hs,l.bindCameraHandlers(),l.mouseCaptor=new Kf(l.mouseLayer,l),l.mouseCaptor.setSettings(l.internals.settings),l.touchCaptor=new Qf(l.mouseLayer,l),l.touchCaptor.setSettings(l.internals.settings),l.bindEventHandlers(),l.bindGraphHandlers(),l.handleSettingsUpdate(),l.refresh(),l}return ie(t,e),Z(t,[{key:"initializeWebGLLabels",value:function(){this.sdfAtlas=new vt,this.sdfAtlas.registerFont({family:"sans-serif",weight:"normal",style:"normal"})}},{key:"resetWebGLTexture",value:function(){var r=this.webGLContext;if(!this.pickingFrameBuffer)return this;var a=Math.ceil(this.width*this.internals.pixelRatio/this.internals.settings.pickingDownSizingRatio),i=Math.ceil(this.height*this.internals.pixelRatio/this.internals.settings.pickingDownSizingRatio);r.bindFramebuffer(r.FRAMEBUFFER,this.pickingFrameBuffer),this.pickingTexture&&r.deleteTexture(this.pickingTexture);var o=r.createTexture();o&&(r.bindTexture(r.TEXTURE_2D,o),r.texImage2D(r.TEXTURE_2D,0,r.RGBA,a,i,0,r.RGBA,r.UNSIGNED_BYTE,null),r.texParameteri(r.TEXTURE_2D,r.TEXTURE_MIN_FILTER,r.NEAREST),r.texParameteri(r.TEXTURE_2D,r.TEXTURE_MAG_FILTER,r.NEAREST),r.framebufferTexture2D(r.FRAMEBUFFER,r.COLOR_ATTACHMENT0,r.TEXTURE_2D,o,0),this.pickingTexture=o),this.pickingDepthBuffer&&r.deleteRenderbuffer(this.pickingDepthBuffer);var s=r.createRenderbuffer();return s&&(r.bindRenderbuffer(r.RENDERBUFFER,s),r.renderbufferStorage(r.RENDERBUFFER,r.DEPTH_COMPONENT16,a,i),r.framebufferRenderbuffer(r.FRAMEBUFFER,r.DEPTH_ATTACHMENT,r.RENDERBUFFER,s),this.pickingDepthBuffer=s),r.bindFramebuffer(r.FRAMEBUFFER,null),this}},{key:"bindCameraHandlers",value:function(){var r=this;return this.activeListeners.camera=function(){r.refreshMatrices(),r.labelRenderer.labelsDirty=!0,r.scheduleRender()},this.activeListeners.cameraAnimationStart=function(a){var i=a.from,o=a.to;i.ratio!==o.ratio&&r.stateManager.setGraphState({isZooming:!0})},this.activeListeners.cameraAnimationEnd=function(){r.stateManager.setGraphState({isZooming:!1})},this.camera.on("updated",this.activeListeners.camera),this.camera.on("animationStart",this.activeListeners.cameraAnimationStart),this.camera.on("animationEnd",this.activeListeners.cameraAnimationEnd),this}},{key:"unbindCameraHandlers",value:function(){return this.camera.removeListener("updated",this.activeListeners.camera),this.camera.removeListener("animationStart",this.activeListeners.cameraAnimationStart),this.camera.removeListener("animationEnd",this.activeListeners.cameraAnimationEnd),this}},{key:"getHitAtPosition",value:function(r){var a;if(this.contextLost)return null;var i=this.webGLContext;i.bindFramebuffer(i.FRAMEBUFFER,this.pickingFrameBuffer);var o=fa(i,this.pickingFrameBuffer,r.x,r.y,this.internals.pixelRatio,this.internals.settings.pickingDownSizingRatio),s=nr.apply(void 0,$(o));return(a=this.pickingState.lookup[s])!==null&&a!==void 0?a:null}},{key:"bindEventHandlers",value:function(){return ug(this.internals,this.mouseCaptor,this.touchCaptor,this.activeListeners),this}},{key:"bindGraphHandlers",value:function(){return cg({graph:this.internals.graph,edgeGroups:this.edgeGroups,addNode:this.addNode.bind(this),updateNode:this.updateNode.bind(this),removeNode:this.removeNode.bind(this),addEdge:this.addEdge.bind(this),updateEdge:this.updateEdge.bind(this),removeEdge:this.removeEdge.bind(this),clearEdgeState:this.clearEdgeState.bind(this),clearNodeState:this.clearNodeState.bind(this),clearEdgeIndices:this.clearEdgeIndices.bind(this),clearNodeIndices:this.clearNodeIndices.bind(this),refresh:this.refresh.bind(this)},this.activeListeners),this}},{key:"unbindGraphHandlers",value:function(){dg(this.internals.graph,this.activeListeners)}},{key:"getNodeShapeId",value:function(r){return this.internals.nodeShapeMap&&this.internals.nodeGlobalShapeIds&&r.shape&&r.shape in this.internals.nodeShapeMap?this.internals.nodeGlobalShapeIds[this.internals.nodeShapeMap[r.shape]]:Ut(r.shape||"circle")}},{key:"processNodes",value:function(){var r=this,a=this.internals.graph,i=this.internals.settings,o=this.getDimensions(),s=i.autoRescale,l=i.autoRescaleContent,u=this.nodeExtent;if(s===!1){var c=o.width,d=o.height;u={x:[-c/2,c/2],y:[-d/2,d/2]}}else(s!=="once"||!this.autoRescaleFrozen)&&(u=this.computeNodeExtent(),l!=="positions"&&!this.customBBox&&(u=pg({extent:u,coords:this.nodeGraphCoords,nodeData:this.internals.nodeDataCache,dimensions:o,stagePadding:this.getStagePadding(),zoomToSizeRatioFunction:i.zoomToSizeRatioFunction,itemSizesReference:i.itemSizesReference,fitLabels:l==="labels",nodeLabelBox:function(j,U){return r.labelRenderer.nodeLabelBox(j,U)}})),s==="once"&&(this.autoRescaleFrozen=!0));this.nodeExtent=u,this.normalizationFunction=hr(this.customBBox||this.nodeExtent);var f=new Hs,h=Ke(f.getState(),o,this.getGraphDimensions(),this.getStagePadding());this.labelRenderer.labelGrid.resizeAndClear(o,i.labelGridCellSize),this.labelRenderer.edgeAnchorGrid.resizeAndClear(o,i.labelGridCellSize);var g=i.renderEdgeLabels&&i.edgeLabelAnchors==="allNodes",m=!1;Ja(this.pickingState,"node");for(var v=a.nodes(),p=0,b=v.length;p<b;p++){var y=v[p],_=this.internals.nodeDataCache[y],x=this.nodeGraphCoords[y];_.x=x.x,_.y=x.y,this.normalizationFunction.applyTo(_),_.visibility!==this.prevNodeVisibilities[y]&&(m=!0),typeof _.label=="string"&&_.visibility!=="hidden"&&_.labelVisibility!=="hidden"&&this.labelRenderer.labelGrid.add(y,_.size,this.framedGraphToViewport(_,{matrix:h})),g&&_.visibility!=="hidden"&&this.labelRenderer.edgeAnchorGrid.add(y,_.size,this.framedGraphToViewport(_,{matrix:h}))}this.labelRenderer.labelGrid.organize(),this.labelRenderer.edgeAnchorGrid.organize(),this.nodeProgram.reallocate(v.length);var T=0;this.depthRanges.nodes={},this.nodeBaseDepth={};var E=this.internals.nodeDataCache,S=G(this.depthLayers),w;try{for(S.s();!(w=S.n()).done;){var A=w.value,C=this.itemBuckets.nodes.getSorted(A,function(M){return E[M].zIndex});if(C.length!==0){this.depthRanges.nodes[A]=[{offset:T,count:C.length}];var D=G(C),F;try{for(D.s();!(F=D.n()).done;){var k=F.value;this.nodeBaseDepth[k]=A,this.nodeProgram.allocateNode(k),Xs(this.pickingState,"node",k),this.addNodeToProgram(k,T++)}}catch(M){D.e(M)}finally{D.f()}}}}catch(M){S.e(M)}finally{S.f()}this.nodeProgram.invalidateBuffers();for(var L=0,z=v.length;L<z;L++)this.prevNodeVisibilities[v[L]]=this.internals.nodeDataCache[v[L]].visibility;this.labelRenderer.processWebGLLabels(v);var N=G(this.customLayerPrograms.values()),P;try{for(N.s();!(P=N.n()).done;){var I=P.value.program;I.cacheData&&I.cacheData()}}catch(M){N.e(M)}finally{N.f()}return m}},{key:"processEdges",value:function(){var r=this.internals.graph,a=r.edges();this.edgeProgram.reallocate(a.length);var i=0;Ja(this.pickingState,"edge"),this.depthRanges.edges={},this.edgeBaseDepth={};var o=this.internals.edgeDataCache,s=G(this.depthLayers),l;try{for(s.s();!(l=s.n()).done;){var u=l.value,c=this.itemBuckets.edges.getSorted(u,function(g){return o[g].zIndex});if(c.length!==0){this.depthRanges.edges[u]=[{offset:i,count:c.length}];var d=G(c),f;try{for(d.s();!(f=d.n()).done;){var h=f.value;this.edgeBaseDepth[h]=u,Xs(this.pickingState,"edge",h),this.addEdgeToProgram(h,i++)}}catch(g){d.e(g)}finally{d.f()}}}}catch(g){s.e(g)}finally{s.f()}this.edgeProgram.invalidateBuffers()}},{key:"updateNodeDepthRanges",value:function(r,a,i){var o=this.nodeProgramIndex[r];o!==void 0&&(Da(this.depthRanges.nodes,a,o),Ca(this.depthRanges.nodes,i,o))}},{key:"updateEdgeDepthRanges",value:function(r,a,i){var o=this.edgeProgramIndex[r];o!==void 0&&(Da(this.depthRanges.edges,a,o),Ca(this.depthRanges.edges,i,o))}},{key:"handleSettingsUpdate",value:function(){var r=this,a=this.internals.settings;return this.camera.minRatio=a.minCameraRatio,this.camera.maxRatio=a.maxCameraRatio,this.camera.enabledZooming=a.enableCameraZooming,this.camera.enabledPanning=a.enableCameraPanning,this.camera.enabledRotation=a.enableCameraRotation,a.cameraPanBoundaries?this.camera.constrainState=function(i){return r.cleanCameraState(i,a.cameraPanBoundaries&&J(a.cameraPanBoundaries)==="object"?a.cameraPanBoundaries:{})}:this.camera.constrainState=null,this.camera.setState(this.camera.getState()),this.mouseLayer.style.touchAction=a.gestureTarget==="graph"?"none":a.gestureTarget==="shared"?"pan-x pan-y":"auto",a.gestureTarget!=="shared"&&this.gestureHint&&(this.gestureHint.kill(),this.gestureHint=null),this.mouseCaptor.setSettings(this.internals.settings),this.touchCaptor.setSettings(this.internals.settings),this}},{key:"cleanCameraState",value:function(r){var a=arguments.length>1&&arguments[1]!==void 0?arguments[1]:{},i=a.tolerance,o=i===void 0?0:i,s=a.boundaries,l=B({},r),u=s||this.nodeExtent,c=Q(u.x,2),d=c[0],f=c[1],h=Q(u.y,2),g=h[0],m=h[1],v=[this.graphToViewport({x:d,y:g},{cameraState:r}),this.graphToViewport({x:f,y:g},{cameraState:r}),this.graphToViewport({x:d,y:m},{cameraState:r}),this.graphToViewport({x:f,y:m},{cameraState:r})],p=1/0,b=-1/0,y=1/0,_=-1/0;v.forEach(function(k){var L=k.x,z=k.y;p=Math.min(p,L),b=Math.max(b,L),y=Math.min(y,z),_=Math.max(_,z)});var x=b-p,T=_-y,E=this.getDimensions(),S=E.width,w=E.height,A=0,C=0;if(x>=S?b<S-o?A=b-(S-o):p>o&&(A=p-o):b>S+o?A=b-(S+o):p<-o&&(A=p+o),T>=w?_<w-o?C=_-(w-o):y>o&&(C=y-o):_>w+o?C=_-(w+o):y<-o&&(C=y+o),A||C){var D=this.viewportToFramedGraph({x:0,y:0},{cameraState:r}),F=this.viewportToFramedGraph({x:A,y:C},{cameraState:r});A=F.x-D.x,C=F.y-D.y,l.x+=A,l.y+=C}return l}},{key:"refreshMatrices",value:function(){var r=this.camera.getState(),a=this.getDimensions(),i=this.getGraphDimensions(),o=this.getStagePadding();this.matrix=Ke(r,a,i,o),this.invMatrix=Ke(r,a,i,o,!0),this.correctionRatio=ha(this.matrix,r,a),this.graphToViewportRatio=this.getGraphToViewportRatio()}},{key:"processData",value:function(){var r;this.emit("beforeProcess"),(r=this.internals.attachmentManager)===null||r===void 0||r.clear();var a=B({},this.pickingState.idsByKind),i=this.processNodes();(this.pendingProcess==="full"||i)&&this.processEdges(),rg(this.pickingState,this.internals),ag(a,this.pickingState.idsByKind)||this.internals.hoverResolver.invalidate(),this.pendingProcess="none",this.emit("afterProcess")}},{key:"render",value:function(){var r=this;if(this.contextLost)return this;this.emit("beforeRender");var a=function(){return r.internals.hoverResolver.frameRendered(),r.emit("afterRender"),r};if(this.renderFrame&&(cancelAnimationFrame(this.renderFrame),this.renderFrame=null),this.resize(),(this.pendingProcess!=="none"||this.needToRefreshState)&&(this.labelRenderer.labelsDirty=!0),this.pendingProcess!=="none"&&this.processData(),this.needToRefreshState&&this.refreshState(),this.needToRefreshState=!1,this.stateManager.clearDirtyTracking(),this.pendingProcess!=="none"&&this.processData(),this.clear(),this.resetWebGLTexture(),!this.internals.graph.order)return a();var i=this.mouseCaptor,o=this.camera.isAnimating()||i.isMoving||i.draggedEvents||i.currentWheelDirection;if(this.refreshMatrices(),this.frameId++,this.internals.settings.renderEdgeLabels&&!this.internals.edgeLabelProgram){var s=this.createEdgeLabelPrograms(),l=s.labelProgram,u=s.labelBackgroundProgram;this.internals.edgeLabelProgram=l,this.internals.edgeLabelBackgroundProgram=u}var c=this.internals.settings.DEBUG_logRenderStats?this.getDebugPrograms():null;c&&c.forEach(function(X){return X.resetDebugStats()}),this.internals.settings.DEBUG_gpuTimerQueries&&this.beginGpuTimerQuery(),this.internals.nodeFrameTexture.ensureCapacity(this.internals.nodeDataTexture.getCapacity()),this.internals.edgeFrameTexture.ensureCapacity(this.internals.edgeDataTexture.getCapacity());var d=this.getRenderParams(),f=ge.edge.writesPickingThisFrame(this.internals)?d:B(B({},d),{},{pickingFrameBuffer:null});this.labelRenderer.resetFrame();var h=this.webGLContext,g=Math.ceil(this.width*this.internals.pixelRatio/this.internals.settings.pickingDownSizingRatio),m=Math.ceil(this.height*this.internals.pixelRatio/this.internals.settings.pickingDownSizingRatio);if(h.bindFramebuffer(h.FRAMEBUFFER,this.pickingFrameBuffer),h.viewport(0,0,g,m),h.clear(h.COLOR_BUFFER_BIT|h.DEPTH_BUFFER_BIT),h.bindFramebuffer(h.FRAMEBUFFER,null),h.viewport(0,0,this.width*this.internals.pixelRatio,this.height*this.internals.pixelRatio),h.clear(h.COLOR_BUFFER_BIT|h.DEPTH_BUFFER_BIT),this.internals.nodeDataTexture.upload(),this.internals.edgeDataTexture.upload(),this.nodeProgram.uploadLayerTexture(),this.edgeProgram.uploadAttributeTexture(),this.emit("afterTexturesUpload"),this.internals.nodeDataTexture.bind(nl),this.internals.edgeDataTexture.bind(rl),this.edgeFramePass.run(d,this.internals.edgeFrameTexture,this.internals.edgeDataTexture.getHighWaterMark(),this.edgeProgram.getAttributeTexture(),this.nodeProgram.getAttributeTexture()),this.internals.edgeFrameTexture.bind(el),this.internals.settings.renderLabels){this.labelRenderer.computeDisplayedNodeLabels();var v=this.labelRenderer.buildFramePassPoints(),p=v.data,b=v.count;this.nodeFramePass.run(p,b,this.internals.nodeFrameTexture,d,this.nodeProgram.getAttributeTexture()),this.internals.nodeFrameTexture.bind(tl)}this.internals.settings.renderEdgeLabels&&this.labelRenderer.computeDisplayedEdgeLabels();var y=G(this.customLayerPrograms.values()),_;try{for(y.s();!(_=y.n()).done;){var x=_.value.program;x.preRender&&x.preRender(d)}}catch(X){y.e(X)}finally{y.f()}h.bindFramebuffer(h.FRAMEBUFFER,null),h.viewport(0,0,this.width*this.internals.pixelRatio,this.height*this.internals.pixelRatio),h.blendFunc(h.ONE,h.ONE_MINUS_SRC_ALPHA);var T=!this.internals.settings.hideLabelsOnMove||!o,E=G(this.depthLayers),S;try{for(E.s();!(S=E.n()).done;){var w=S.value,A=G(this.customLayerPrograms.values()),C;try{for(A.s();!(C=A.n()).done;){var D=C.value;D.depth===w&&D.program.render(d)}}catch(X){A.e(X)}finally{A.f()}var F=this.depthRanges.edges[w];if(F&&(!this.internals.settings.hideEdgesOnMove||!o)){var k=G(F),L;try{for(k.s();!(L=k.n()).done;){var z=L.value,N=z.offset,P=z.count;P>0&&this.edgeProgram.render(f,N,P)}}catch(X){k.e(X)}finally{k.f()}}this.internals.settings.renderEdgeLabels&&T&&(this.labelRenderer.renderEdgeLabelBackgrounds(ge.edgeLabel.writesPickingThisFrame(this.internals)?d:B(B({},d),{},{pickingFrameBuffer:null}),w),this.labelRenderer.renderEdgeLabels(d,w)),this.labelRenderer.cacheAttachments(w),this.labelRenderer.renderBackdrops(B(B({},d),{},{pickingFrameBuffer:null}),w);var I=this.depthRanges.nodes[w];if(I){var M=G(I),j;try{for(M.s();!(j=M.n()).done;){var U=j.value,Y=U.offset,q=U.count;q>0&&this.nodeProgram.render(d,Y,q)}}catch(X){M.e(X)}finally{M.f()}}T&&(this.labelRenderer.renderAttachments(B(B({},d),{},{pickingFrameBuffer:null}),w),this.labelRenderer.renderLabelBackgrounds(ge.nodeLabel.writesPickingThisFrame(this.internals)?d:B(B({},d),{},{pickingFrameBuffer:null}),w),this.internals.settings.renderLabels&&this.labelRenderer.renderWebGLLabels(d,w))}}catch(X){E.e(X)}finally{E.f()}return this.labelRenderer.labelsDirty=!1,this.internals.settings.DEBUG_displayPickingLayer&&(h.bindFramebuffer(h.READ_FRAMEBUFFER,this.pickingFrameBuffer),h.bindFramebuffer(h.DRAW_FRAMEBUFFER,null),h.blitFramebuffer(0,0,g,m,0,0,this.width*this.internals.pixelRatio,this.height*this.internals.pixelRatio,h.COLOR_BUFFER_BIT,h.NEAREST)),this.internals.settings.DEBUG_gpuTimerQueries&&this.endGpuTimerQuery(),this.pollGpuTimerQueries(),c&&this.logRenderStats(c),a()}},{key:"getDebugPrograms",value:function(){return[this.nodeProgram,this.edgeProgram,this.internals.labelProgram,this.internals.edgeLabelProgram,this.internals.edgeLabelBackgroundProgram,this.internals.backdropProgram,this.internals.labelBackgroundProgram,this.internals.attachmentProgram].filter(function(r){return r!==null})}},{key:"logRenderStats",value:function(r){var a={},i=G(r),o;try{for(i.s();!(o=i.n()).done;){var s=o.value;a[s.constructor.name]=B({},s.debugStats)}}catch(l){i.e(l)}finally{i.f()}console.log("[sigma] DEBUG_logRenderStats: frame #".concat(this.frameId)),console.table(a)}},{key:"getGpuTimerExtension",value:function(){return this.gpuTimerExt===void 0&&(this.gpuTimerExt=this.webGLContext.getExtension("EXT_disjoint_timer_query_webgl2"),this.gpuTimerExt||console.warn("Sigma: DEBUG_gpuTimerQueries is enabled, but this browser/driver doesn't support EXT_disjoint_timer_query_webgl2.")),this.gpuTimerExt}},{key:"beginGpuTimerQuery",value:function(){var r=this.getGpuTimerExtension();if(r){var a=this.webGLContext,i=a.createQuery();i&&(a.beginQuery(r.TIME_ELAPSED_EXT,i),this.activeGpuTimerQuery=i)}}},{key:"endGpuTimerQuery",value:function(){!this.gpuTimerExt||!this.activeGpuTimerQuery||(this.webGLContext.endQuery(this.gpuTimerExt.TIME_ELAPSED_EXT),this.pendingGpuTimerQueries.push({query:this.activeGpuTimerQuery,frameId:this.frameId}),this.activeGpuTimerQuery=null)}},{key:"pollGpuTimerQueries",value:function(){if(this.pendingGpuTimerQueries.length!==0){var r=this.webGLContext,a=this.gpuTimerExt;if(!a){this.pendingGpuTimerQueries.forEach(function(d){var f=d.query;return r.deleteQuery(f)}),this.pendingGpuTimerQueries=[];return}var i=r.getParameter(a.GPU_DISJOINT_EXT),o=[],s=G(this.pendingGpuTimerQueries),l;try{for(s.s();!(l=s.n()).done;){var u=l.value;if(!r.getQueryParameter(u.query,r.QUERY_RESULT_AVAILABLE)){o.push(u);continue}if(!i){var c=r.getQueryParameter(u.query,r.QUERY_RESULT);console.log("[sigma] DEBUG_gpuTimerQueries: frame #".concat(u.frameId," GPU time = ").concat((c/1e6).toFixed(2),"ms"))}r.deleteQuery(u.query)}}catch(d){s.e(d)}finally{s.f()}this.pendingGpuTimerQueries=o}}},{key:"postEvaluateNode",value:function(r,a,i){var o=r;o.x===void 0&&(o.x=a.x),o.y===void 0&&(o.y=a.y),r.highlighted=i.isHighlighted;for(var s=0,l=this.nodeVariableEntries.length;s<l;s++){var u,c,d=Q(this.nodeVariableEntries[s],2),f=d[0],h=d[1];o[f]=(u=(c=o[f])!==null&&c!==void 0?c:a[f])!==null&&u!==void 0?u:h.default}}},{key:"addNode",value:function(r){var a=this.internals.graph.getNodeAttributes(r),i=this.stateManager.getNodeState(r),o=this.internals.nodeDataCache[r]||{};if(Ea(this.stylesDeclaration.nodes,a,i,this.stateManager.graphState,this.internals.graph,o),this.postEvaluateNode(o,a,i),this.nodeReducer){var s=this.nodeReducer(r,o,a,i,this.stateManager.graphState,this.internals.graph);o=B(B({},o),s)}if(typeof o.x!="number"||typeof o.y!="number")throw new Error('Sigma: could not find a valid position (x, y) for node "'.concat(r,'". ')+"Provide coordinates via node attributes, styles, or a nodeReducer.");this.internals.nodeShapeMap?(!o.shape||!(o.shape in this.internals.nodeShapeMap))&&(o.shape=Object.keys(this.internals.nodeShapeMap)[0]):this.nodeShapeSlug&&(o.shape=this.nodeShapeSlug),this.internals.nodeDataCache[r]=o,this.nodeGraphCoords[r]={x:o.x,y:o.y},pt(this.internals.nodesWithForcedLabels,r,Qe(o)),pt(this.internals.nodesWithBackdrop,r,wa(o)),this.itemBuckets.nodes.set(r,o.depth)}},{key:"updateNode",value:function(r){this.addNode(r);var a=this.internals.nodeDataCache[r];this.normalizationFunction.applyTo(a)}},{key:"removeNode",value:function(r){this.itemBuckets.nodes.remove(r),delete this.internals.nodeDataCache[r],delete this.nodeGraphCoords[r],delete this.nodeProgramIndex[r],this.internals.dragManager.removeNode(r),this.stateManager.removeNode(r),this.internals.nodesWithForcedLabels.delete(r),this.internals.nodesWithBackdrop.delete(r)}},{key:"postEvaluateEdge",value:function(r,a){for(var i=r,o=0,s=this.edgeVariableEntries.length;o<s;o++){var l,u,c=Q(this.edgeVariableEntries[o],2),d=c[0],f=c[1];i[d]=(l=(u=i[d])!==null&&u!==void 0?u:a[d])!==null&&l!==void 0?l:f.default}}},{key:"applyEdgeSpread",value:function(r,a,i){var o;if(!(i.parallelCount<=1)){var s=this.internals.graph.source(r),l=this.internals.graph.target(r),u=s===l,c=u?a.selfLoopPath||a.path:a.parallelPath||a.path,d=c?this.edgePathsByName.get(c):void 0;if(d!=null&&d.spread){var f=(o=a.parallelSpread)!==null&&o!==void 0?o:.25,h=d.spread.compute(i.parallelIndex,i.parallelCount,f);!u&&this.internals.graph.isDirected(r)&&s>l&&(h=-h),a[d.spread.variable]=h}}}},{key:"addEdge",value:function(r){var a=this.internals.graph.getEdgeAttributes(r),i=this.stateManager.getEdgeState(r),o={};if(Sa(this.stylesDeclaration.edges,a,i,this.stateManager.graphState,this.internals.graph,o),this.postEvaluateEdge(o,a),this.edgeReducer){var s=this.edgeReducer(r,o,a,i,this.stateManager.graphState,this.internals.graph);o=B(B({},o),s)}this.applyEdgeSpread(r,o,i),this.internals.edgeDataCache[r]=o,pt(this.internals.edgesWithForcedLabels,r,Qe(o)),this.itemBuckets.edges.set(r,o.depth)}},{key:"updateEdge",value:function(r){this.addEdge(r)}},{key:"removeEdge",value:function(r){this.itemBuckets.edges.remove(r),delete this.internals.edgeDataCache[r],delete this.edgeProgramIndex[r],delete this.edgeTextureIndexCache[r],this.internals.edgeDataTexture.free(r),this.stateManager.removeEdge(r),this.internals.edgesWithForcedLabels.delete(r)}},{key:"clearNodeIndices",value:function(){this.labelRenderer.resetLabelGrid(),this.autoRescaleFrozen||(this.nodeExtent={x:[0,1],y:[0,1]}),this.internals.nodeDataCache={},this.nodeGraphCoords={},this.edgeProgramIndex={},this.internals.nodesWithForcedLabels.clear(),this.internals.nodesWithBackdrop.clear(),this.prevNodeVisibilities={},this.itemBuckets.nodes.clearAll(),this.depthRanges.nodes={},this.nodeBaseDepth={}}},{key:"clearEdgeIndices",value:function(){this.internals.edgeDataCache={},this.edgeProgramIndex={},this.edgeTextureIndexCache={},this.internals.edgesWithForcedLabels.clear(),Ja(this.pickingState,"edge"),this.itemBuckets.edges.clearAll(),this.depthRanges.edges={},this.edgeBaseDepth={},this.edgeGroups.clear()}},{key:"clearIndices",value:function(){this.clearEdgeIndices(),this.clearNodeIndices()}},{key:"clearNodeState",value:function(){this.labelRenderer.resetFrame(),this.internals.nodesWithBackdrop.clear(),this.internals.dragManager.clear(),this.autoRescaleFrozen=!1,this.stateManager.clearNodes()}},{key:"clearEdgeState",value:function(){this.labelRenderer.clearEdgeLabels(),this.stateManager.clearEdges()}},{key:"clearState",value:function(){this.clearEdgeState(),this.clearNodeState(),this.stateManager.resetGraphState()}},{key:"addNodeToProgram",value:function(r,a){var i,o=this.internals.nodeDataCache[r];this.internals.nodeDataTexture.allocate(r),(i=this.internals.nodeDataTexture).updateNode.apply(i,[r,o.x,o.y,o.size,this.getNodeShapeId(o)].concat($(dr(o)),[o.color]));var s=this.internals.nodeDataTexture.getIndex(r);this.nodeProgram.process(Tt(this.pickingState,"node",r),a,o,s,r),this.nodeProgramIndex[r]=a}},{key:"addEdgeToProgram",value:function(r,a){var i,o,s=this.internals.edgeDataCache[r],l=this.internals.graph.source(r),u=this.internals.graph.target(r),c=this.internals.edgeDataTexture.allocate(r);this.edgeTextureIndexCache[r]=c;var d=l===u,f=!d&&((i=(o=this.stateManager.getEdgeState(r))===null||o===void 0?void 0:o.parallelCount)!==null&&i!==void 0?i:1)>1,h=this.edgeProgram.resolveEdgeIds(s,d,f),g=h.pathId,m=h.headId,v=h.tailId,p=h.headLengthRatio,b=h.tailLengthRatio;this.internals.edgeDataTexture.updateEdge(r,this.internals.nodeDataTexture.getIndex(l),this.internals.nodeDataTexture.getIndex(u),s.size,p,b,g,m,v),this.edgeProgram.process(Tt(this.pickingState,"edge",r),a,this.internals.nodeDataCache[l],this.internals.nodeDataCache[u],s,c),this.edgeProgramIndex[r]=a}},{key:"getRenderParams",value:function(){return{frameId:this.frameId,matrix:this.matrix,invMatrix:this.invMatrix,width:this.width,height:this.height,pixelRatio:this.internals.pixelRatio,zoomRatio:this.camera.ratio,cameraAngle:this.camera.angle,sizeRatio:1/this.scaleSize(),correctionRatio:this.correctionRatio,downSizingRatio:this.internals.settings.pickingDownSizingRatio,minEdgeThickness:this.internals.settings.minEdgeThickness,antiAliasingFeather:this.internals.settings.antiAliasingFeather,nodePickingPadding:this.internals.settings.nodePickingPadding,edgePickingPadding:this.internals.settings.edgePickingPadding,labelPickingPadding:this.internals.settings.labelPickingPadding,nodeDataTextureUnit:nl,nodeDataTextureWidth:this.internals.nodeDataTexture.getTextureWidth(),nodeFrameTextureUnit:tl,nodeFrameTextureWidth:this.internals.nodeFrameTexture.getTextureWidth(),edgeDataTextureUnit:rl,edgeDataTextureWidth:this.internals.edgeDataTexture.getTextureWidth(),edgeFrameTextureUnit:el,edgeFrameTextureWidth:this.internals.edgeFrameTexture.getTextureWidth(),pickingFrameBuffer:this.pickingFrameBuffer,labelPixelSnapping:this.internals.settings.labelPixelSnapping?1:0}}},{key:"getStagePadding",value:function(){var r=this.internals.settings,a=r.stagePadding,i=r.autoRescale;return i&&a||0}},{key:"getLayerElement",value:function(r){if(r==="mouse")return this.mouseLayer;var a=this.extraElements[r];if(!a)throw new Error('Sigma: layer "'.concat(r,'" does not exist'));return a}},{key:"initWebGLContext",value:function(){var r=this,a=this.createWebGLContext("stage");this.stageCanvas=this.extraElements.stage,this.webGLContext=a,this.stageCanvas.addEventListener("webglcontextlost",function(i){r.webGLContext&&(i.preventDefault(),r.contextLost=!0,r.renderFrame&&(cancelAnimationFrame(r.renderFrame),r.renderFrame=null),r.gpuTimerExt=void 0,r.activeGpuTimerQuery=null,r.pendingGpuTimerQueries=[],r.emit("webglContextLost"))}),this.stageCanvas.addEventListener("webglcontextrestored",function(){requestAnimationFrame(function(){r.webGLContext&&!r.webGLContext.isContextLost()&&r.restoreWebGLContext()})}),this.initPickingFramebuffer()}},{key:"initPickingFramebuffer",value:function(){var r=this.webGLContext,a=r.createFramebuffer();if(!a)throw new Error("Sigma: cannot create picking frame buffer");r.bindFramebuffer(r.FRAMEBUFFER,a);var i=r.createTexture();if(!i)throw new Error("Sigma: cannot create picking texture");r.bindTexture(r.TEXTURE_2D,i),r.texImage2D(r.TEXTURE_2D,0,r.RGBA,1,1,0,r.RGBA,r.UNSIGNED_BYTE,null),r.texParameteri(r.TEXTURE_2D,r.TEXTURE_MIN_FILTER,r.NEAREST),r.texParameteri(r.TEXTURE_2D,r.TEXTURE_MAG_FILTER,r.NEAREST),r.texParameteri(r.TEXTURE_2D,r.TEXTURE_WRAP_S,r.CLAMP_TO_EDGE),r.texParameteri(r.TEXTURE_2D,r.TEXTURE_WRAP_T,r.CLAMP_TO_EDGE),r.framebufferTexture2D(r.FRAMEBUFFER,r.COLOR_ATTACHMENT0,r.TEXTURE_2D,i,0);var o=r.createRenderbuffer();if(!o)throw new Error("Sigma: cannot create picking depth buffer");if(r.bindRenderbuffer(r.RENDERBUFFER,o),r.renderbufferStorage(r.RENDERBUFFER,r.DEPTH_COMPONENT16,1,1),r.framebufferRenderbuffer(r.FRAMEBUFFER,r.DEPTH_ATTACHMENT,r.RENDERBUFFER,o),r.checkFramebufferStatus(r.FRAMEBUFFER)!==r.FRAMEBUFFER_COMPLETE)throw new Error("Sigma: picking framebuffer is not complete");r.bindFramebuffer(r.FRAMEBUFFER,null),this.pickingFrameBuffer=a,this.pickingTexture=i,this.pickingDepthBuffer=o}},{key:"initPrograms",value:function(r,a){var i=this,o=this.webGLContext,s=Fs(o,this.pickingFrameBuffer,i,r?.nodes,a.antialiasNodes),l=s.nodeProgram,u=s.labelProgram,c=s.backdropProgram,d=s.labelBackgroundProgram,f=s.attachmentProgram,h=s.framePass,g=s.shapeSlug,m=s.shapeNameToIndex,v=s.shapeGlobalIds,p=s.variables;this.nodeProgram=l,this.nodeFramePass=h,this.nodeVariableEntries=Object.entries(p),g&&(this.nodeShapeSlug=g);var b=Ps(o,this.pickingFrameBuffer,i,r?.edges,a.antialiasEdges,r?.nodes),y=b.edgeProgram,_=b.createLabelPrograms,x=b.framePass,T=b.variables,E=b.paths;return this.createEdgeLabelPrograms=_,this.edgeProgram=y,this.edgeFramePass=x,this.edgeVariableEntries=Object.entries(T),this.edgePathsByName=new Map(E.map(function(S){return[S.name,S]})),{labelProgram:u,edgeLabelProgram:null,edgeLabelBackgroundProgram:null,backdropProgram:c,labelBackgroundProgram:d,attachmentProgram:f,nodeShapeMap:m??null,nodeGlobalShapeIds:v??null}}},{key:"restoreWebGLContext",value:function(){var r,a,i,o,s,l=this.internals;this.initPickingFramebuffer(),(r=l.nodeDataTexture)===null||r===void 0||r.restore(),(a=l.nodeFrameTexture)===null||a===void 0||a.restore(),(i=l.edgeDataTexture)===null||i===void 0||i.restore(),(o=l.edgeFrameTexture)===null||o===void 0||o.restore(),(s=l.attachmentManager)===null||s===void 0||s.restore(),l.hoverResolver.reset(),Object.assign(l,this.initPrograms(l.primitives,l.settings));var u=G(this.customLayerPrograms.values()),c;try{for(u.s();!(c=u.n()).done;){var d=c.value;d.program.kill(),d.program=d.factory(this.webGLContext)}}catch(f){u.e(f)}finally{u.f()}this.contextLost=!1,this.emit("webglContextRestored"),this.refresh()}},{key:"createLayer",value:function(r,a){var i=arguments.length>2&&arguments[2]!==void 0?arguments[2]:{};if(this.extraElements[r])throw new Error('Sigma: a layer named "'.concat(r,'" already exists'));var o=Wt(a,{position:"absolute"},{class:"sigma-".concat(r)});return i.style&&Object.assign(o.style,i.style),this.extraElements[r]=o,"beforeLayer"in i&&i.beforeLayer?this.getLayerElement(i.beforeLayer).before(o):"afterLayer"in i&&i.afterLayer?this.getLayerElement(i.afterLayer).after(o):this.container.appendChild(o),o}},{key:"createCanvas",value:function(r){var a=arguments.length>1&&arguments[1]!==void 0?arguments[1]:{};return this.createLayer(r,"canvas",a)}},{key:"createWebGLContext",value:function(r){var a=arguments.length>1&&arguments[1]!==void 0?arguments[1]:{},i=this.createCanvas(r,a);a.hidden&&i.remove();var o=i.getContext("webgl2",B({preserveDrawingBuffer:!1,antialias:!1,depth:!0},a));if(!o)throw new Error("Sigma: WebGL 2 is not supported by your browser. Please use a modern browser (Chrome 56+, Firefox 51+, Safari 15+, Edge 79+).");return o.blendFunc(o.ONE,o.ONE_MINUS_SRC_ALPHA),o}},{key:"killLayer",value:function(r){if(r==="stage"||r==="mouse")throw new Error('Sigma: cannot kill built-in layer "'.concat(r,'"'));var a=this.extraElements[r];if(!a)throw new Error("Sigma: cannot kill layer ".concat(r,", which does not exist"));return a.remove(),delete this.extraElements[r],this}},{key:"getWebGLContext",value:function(){if(!this.webGLContext)throw new Error("Sigma: WebGL context is not available");return this.webGLContext}},{key:"getNodeDataTexture",value:function(){if(!this.internals.nodeDataTexture)throw new Error("Sigma: node data texture is not available");return this.internals.nodeDataTexture}},{key:"getNormalizationFunction",value:function(){return this.normalizationFunction}},{key:"addCustomLayerProgram",value:function(r,a,i){var o;if(!this.depthLayers.includes(a))throw new Error('Sigma: cannot add custom layer program at depth "'.concat(a,'", ')+"it must be declared in primitives.depthLayers. Current layers: ".concat(this.depthLayers.join(", ")));(o=this.customLayerPrograms.get(r))===null||o===void 0||o.program.kill();var s=i(this.webGLContext);return this.customLayerPrograms.set(r,{depth:a,factory:i,program:s}),s.cacheData&&s.cacheData(),this.scheduleRender(),this}},{key:"removeCustomLayerProgram",value:function(r){var a=this.customLayerPrograms.get(r);return a&&(a.program.kill(),this.customLayerPrograms.delete(r),this.scheduleRender()),this}},{key:"getCamera",value:function(){return this.camera}},{key:"setCamera",value:function(r){return this.camera.cancelAnimation(),this.unbindCameraHandlers(),this.camera=r,this.bindCameraHandlers(),this.scheduleRender(),this}},{key:"getContainer",value:function(){return this.container}},{key:"getGraph",value:function(){return this.internals.graph}},{key:"setGraph",value:function(r){return r===this.internals.graph?this:(this.stateManager.pruneNodes(function(a){return r.hasNode(a)}),this.stateManager.pruneEdges(function(a){return r.hasEdge(a)}),this.unbindGraphHandlers(),this.checkEdgesEventsFrame!==null&&(cancelAnimationFrame(this.checkEdgesEventsFrame),this.checkEdgesEventsFrame=null),this.internals.graph=r,this.autoRescaleFrozen=!1,this.bindGraphHandlers(),this.refresh(),this)}},{key:"getMouseCaptor",value:function(){return this.mouseCaptor}},{key:"getTouchCaptor",value:function(){return this.touchCaptor}},{key:"getDimensions",value:function(){return{width:this.width,height:this.height}}},{key:"getGraphDimensions",value:function(){var r=this.customBBox||this.nodeExtent;return{width:r.x[1]-r.x[0]||1,height:r.y[1]-r.y[0]||1}}},{key:"getNodeDisplayData",value:function(r){var a=this.internals.nodeDataCache[r];return a?Object.assign({},a):void 0}},{key:"getEdgeDisplayData",value:function(r){var a=this.internals.edgeDataCache[r];return a?Object.assign({},a):void 0}},{key:"getNodeState",value:function(r){return this.stateManager.getNodeState(r)}},{key:"getEdgeState",value:function(r){return this.stateManager.getEdgeState(r)}},{key:"getGraphState",value:function(){return this.stateManager.getGraphState()}},{key:"setNodeState",value:function(r,a){return this.stateManager.setNodeState(r,a),this}},{key:"setEdgeState",value:function(r,a){return this.stateManager.setEdgeState(r,a),this}},{key:"setGraphState",value:function(r){return this.stateManager.setGraphState(r),this}},{key:"_setPanning",value:function(r){this.stateManager.setGraphState({isPanning:r})}},{key:"_setZooming",value:function(r){this.stateManager.setGraphState({isZooming:r})}},{key:"_hasNodeDrag",value:function(){var r=this.internals.dragManager;return!!(r.pendingNode||r.session)}},{key:"_showGestureHint",value:function(r){var a=this.internals.settings;a.gestureTarget==="shared"&&(this.gestureHint=this.gestureHint||new _g(this.container),this.gestureHint.show(r,a))}},{key:"setNodesState",value:function(r,a){return this.stateManager.setNodesState(r,a),this}},{key:"setEdgesState",value:function(r,a){return this.stateManager.setEdgesState(r,a),this}},{key:"updateContainerCursor",value:function(){var r=this.stateManager.hovered,a=this.resolvedStageStyle.cursor||"";this.container.style.cursor=r&&ig(this.internals,r)||a}},{key:"refreshStageStyle",value:function(){this.resolvedStageStyle=Ra(this.stylesDeclaration.stage,this.stateManager.graphState),this.resolvedStageStyle.background!==void 0&&(this.container.style.backgroundColor=this.resolvedStageStyle.background),this.updateContainerCursor()}},{key:"getNodeDisplayedLabels",value:function(){return new Set(this.labelRenderer.displayedNodeLabels)}},{key:"getEdgeDisplayedLabels",value:function(){return new Set(this.labelRenderer.displayedEdgeLabels)}},{key:"getSettings",value:function(){return B({},this.internals.settings)}},{key:"getSetting",value:function(r){return this.internals.settings[r]}},{key:"getStyles",value:function(){return B({},this.stylesDeclaration)}},{key:"getPrimitives",value:function(){return B({},this.internals.primitives)}},{key:"setSetting",value:function(r,a){return this.internals.settings[r]=a,gr(this.internals.settings),this.handleSettingsUpdate(),this.scheduleRefresh(),this}},{key:"updateSetting",value:function(r,a){return this.setSetting(r,a(this.internals.settings[r])),this}},{key:"setSettings",value:function(r){return this.internals.settings=B(B({},this.internals.settings),r),gr(this.internals.settings),this.handleSettingsUpdate(),this.scheduleRefresh(),this}},{key:"resize",value:function(r){var a=this.width,i=this.height;if(this.width=this.container.offsetWidth,this.height=this.container.offsetHeight,this.internals.pixelRatio=gn(),this.width===0)if(this.internals.settings.allowInvalidContainer)this.width=1;else throw new Error("Sigma: Container has no width. You can set the allowInvalidContainer setting to true to stop seeing this error.");if(this.height===0)if(this.internals.settings.allowInvalidContainer)this.height=1;else throw new Error("Sigma: Container has no height. You can set the allowInvalidContainer setting to true to stop seeing this error.");if(!r&&a===this.width&&i===this.height)return this;this.labelRenderer.labelsDirty=!0;for(var o=0,s=[this.mouseLayer].concat($(Object.values(this.extraElements)));o<s.length;o++){var l=s[o];l.style.width=this.width+"px",l.style.height=this.height+"px"}return this.webGLContext&&(this.stageCanvas.setAttribute("width",this.width*this.internals.pixelRatio+"px"),this.stageCanvas.setAttribute("height",this.height*this.internals.pixelRatio+"px"),this.webGLContext.viewport(0,0,this.width*this.internals.pixelRatio,this.height*this.internals.pixelRatio)),this.emit("resize"),this}},{key:"clear",value:function(){return this.emit("beforeClear"),this.webGLContext.bindFramebuffer(WebGLRenderingContext.FRAMEBUFFER,null),this.webGLContext.clear(WebGLRenderingContext.COLOR_BUFFER_BIT),this.emit("afterClear"),this}},{key:"scheduleStateRefresh",value:function(){this.needToRefreshState=!0,this.scheduleRender()}},{key:"refreshState",value:function(){var r=this,a;this.stateManager.flushGraphStateFlags();var i=this.stateManager.graphStateChanged&&this.internals.nodeStyleAnalysis.dependency==="graph-state",o=this.stateManager.graphStateChanged&&this.edgeStyleAnalysis.dependency==="graph-state",s=!1;if(i)this.internals.graph.forEachNode(function(g){r.refreshNodeState(g)&&(s=!0)});else if(this.internals.nodeStyleAnalysis.dependency!=="static"){var l=G(this.stateManager.dirtyNodes),u;try{for(l.s();!(u=l.n()).done;){var c=u.value;this.refreshNodeState(c)&&(s=!0)}}catch(g){l.e(g)}finally{l.f()}}if(o)this.internals.graph.forEachEdge(function(g){r.refreshEdgeState(g)&&(s=!0)});else if(this.edgeStyleAnalysis.dependency!=="static"){var d=G(this.stateManager.dirtyEdges),f;try{for(d.s();!(f=d.n()).done;){var h=f.value;this.refreshEdgeState(h)&&(s=!0)}}catch(g){d.e(g)}finally{d.f()}}this.stateManager.graphStateChanged&&(a=this.stylesDeclaration)!==null&&a!==void 0&&a.stage&&this.refreshStageStyle(),this.stateManager.clearDirtyTracking(),s&&(this.pendingProcess="full")}},{key:"refreshNodeState",value:function(r){var a=this.internals.nodeDataCache[r];if(!a||this.nodeReducer){var i,o,s,l,u=(i=this.internals.nodeDataCache[r])===null||i===void 0?void 0:i.depth,c=(o=this.internals.nodeDataCache[r])===null||o===void 0?void 0:o.zIndex,d=(s=this.internals.nodeDataCache[r])===null||s===void 0?void 0:s.labelAttachment;this.updateNode(r);var f=this.internals.nodeDataCache[r];this.internals.attachmentManager&&f.labelAttachment!==d&&this.internals.attachmentManager.invalidateNode(r);var h;this.internals.nodeShapeMap&&this.internals.nodeGlobalShapeIds&&f.shape&&f.shape in this.internals.nodeShapeMap?h=this.internals.nodeGlobalShapeIds[this.internals.nodeShapeMap[f.shape]]:h=Ut(f.shape||"circle"),(l=this.internals.nodeDataTexture).updateNode.apply(l,[r,f.x,f.y,f.size,h].concat($(dr(f)),[f.color])),u&&f.depth!==u&&this.updateNodeDepthRanges(r,u,f.depth);var g=this.nodeProgramIndex[r];return g!==void 0&&(this.addNodeToProgram(r,g),this.nodeProgram.invalidateBuffers()),c!==void 0&&f.zIndex!==c}var m=this.internals.graph.getNodeAttributes(r),v=this.stateManager.getNodeState(r),p=a.size,b=a.shape,y=a.depth,_=a.zIndex,x=a.labelAttachment,T=a.rotationAlignment,E=a.labelRotationAlignment,S=a.color;Ea(this.stylesDeclaration.nodes,m,v,this.stateManager.graphState,this.internals.graph,a),this.postEvaluateNode(a,m,v);var w=this.nodeGraphCoords[r],A=a.x!==w.x||a.y!==w.y;if(A&&(w.x=a.x,w.y=a.y),this.normalizationFunction.applyTo(a),this.internals.nodeShapeMap?(!a.shape||!(a.shape in this.internals.nodeShapeMap))&&(a.shape=Object.keys(this.internals.nodeShapeMap)[0]):this.nodeShapeSlug&&(a.shape=this.nodeShapeSlug),this.internals.attachmentManager&&a.labelAttachment!==x&&this.internals.attachmentManager.invalidateNode(r),pt(this.internals.nodesWithForcedLabels,r,Qe(a)),pt(this.internals.nodesWithBackdrop,r,wa(a)),A||a.size!==p||a.shape!==b||a.rotationAlignment!==T||a.labelRotationAlignment!==E||a.color!==S){var C,D;this.internals.nodeShapeMap&&this.internals.nodeGlobalShapeIds&&a.shape&&a.shape in this.internals.nodeShapeMap?D=this.internals.nodeGlobalShapeIds[this.internals.nodeShapeMap[a.shape]]:D=Ut(a.shape||"circle"),(C=this.internals.nodeDataTexture).updateNode.apply(C,[r,a.x,a.y,a.size,D].concat($(dr(a)),[a.color]))}this.itemBuckets.nodes.set(r,a.depth),a.depth!==y&&this.updateNodeDepthRanges(r,y,a.depth);var F=this.nodeProgramIndex[r];return F!==void 0&&(this.addNodeToProgram(r,F),this.nodeProgram.invalidateBuffers()),a.zIndex!==_}},{key:"refreshEdgeState",value:function(r){var a=this.internals.edgeDataCache[r];if(!a||this.edgeReducer){var i=a?.depth,o=a?.zIndex;this.updateEdge(r);var s=this.internals.edgeDataCache[r];i&&s.depth!==i&&this.updateEdgeDepthRanges(r,i,s.depth);var l=this.edgeProgramIndex[r];return l!==void 0&&(this.addEdgeToProgram(r,l),this.edgeProgram.invalidateBuffers()),o!==void 0&&s.zIndex!==o}var u=this.internals.graph.getEdgeAttributes(r),c=this.stateManager.getEdgeState(r),d=a.depth,f=a.zIndex,h=a.size,g=a.path,m=a.selfLoopPath,v=a.parallelPath,p=a.head,b=a.tail;Sa(this.stylesDeclaration.edges,u,c,this.stateManager.graphState,this.internals.graph,a),this.postEvaluateEdge(a,u),this.applyEdgeSpread(r,a,c),pt(this.internals.edgesWithForcedLabels,r,Qe(a)),this.itemBuckets.edges.set(r,a.depth),a.depth!==d&&this.updateEdgeDepthRanges(r,d,a.depth);var y=this.edgeProgramIndex[r];if(y!==void 0){var _=a.size!==h||a.path!==g||a.selfLoopPath!==m||a.parallelPath!==v||a.head!==p||a.tail!==b;if(_)this.addEdgeToProgram(r,y),this.edgeProgram.invalidateBuffers();else{var x=this.internals.graph.source(r),T=this.internals.graph.target(r),E=this.internals.nodeDataCache[x],S=this.internals.nodeDataCache[T],w=this.edgeTextureIndexCache[r];this.edgeProgram.process(Tt(this.pickingState,"edge",r),y,E,S,a,w),this.edgeProgram.invalidateBuffers()}}return a.zIndex!==f}},{key:"refresh",value:function(r){var a=this,i=r?.skipIndexation!==void 0?r?.skipIndexation:!1,o=r?.schedule!==void 0?r.schedule:!1,s=!r||!r.partialGraph;if(s)this.clearEdgeIndices(),this.clearNodeIndices(),this.internals.graph.forEachNode(function(E){return a.addNode(E)}),this.edgeGroups.rebuild(),this.internals.graph.forEachEdge(function(E){return a.addEdge(E)}),this.pendingProcess="full";else{for(var l,u,c=((l=r.partialGraph)===null||l===void 0?void 0:l.nodes)||[],d=0,f=c?.length||0;d<f;d++){var h,g,m=""+c[d],v=(h=this.internals.nodeDataCache[m])===null||h===void 0?void 0:h.labelAttachment;if(this.updateNode(m),this.internals.attachmentManager&&((g=this.internals.nodeDataCache[m])!==null&&g!==void 0&&g.labelAttachment||v)&&this.internals.attachmentManager.invalidateNode(m),i){var p=this.nodeProgramIndex[m];if(p===void 0)throw new Error('Sigma: node "'.concat(m,`" can't be repaint`));this.addNodeToProgram(m,p)}}i&&c.length>0&&(this.nodeProgram.invalidateBuffers(),this.labelRenderer.labelsDirty=!0);for(var b=(r==null||(u=r.partialGraph)===null||u===void 0?void 0:u.edges)||[],y=0,_=b.length;y<_;y++){var x=""+b[y];if(this.updateEdge(x),i){var T=this.edgeProgramIndex[x];if(T===void 0)throw new Error('Sigma: edge "'.concat(x,`" can't be repaint`));this.addEdgeToProgram(x,T)}}i&&b.length>0&&(this.edgeProgram.invalidateBuffers(),this.labelRenderer.labelsDirty=!0),!i&&this.pendingProcess!=="full"&&(this.pendingProcess=b.length>0?"full":"nodes")}return o?this.scheduleRender():this.render(),this}},{key:"scheduleRender",value:function(){var r=this;return this.renderFrame||(this.renderFrame=requestAnimationFrame(function(){r.render()})),this}},{key:"scheduleRefresh",value:function(r){return this.refresh(B(B({},r),{},{schedule:!0}))}},{key:"getViewportZoomedState",value:function(r,a){var i=this.camera.getState(),o=i.ratio,s=i.angle,l=i.x,u=i.y,c=this.internals.settings,d=c.minCameraRatio,f=c.maxCameraRatio;typeof f=="number"&&(a=Math.min(a,f)),typeof d=="number"&&(a=Math.max(a,d));var h=a/o,g={x:this.width/2,y:this.height/2},m=this.viewportToFramedGraph(r),v=this.viewportToFramedGraph(g);return{angle:s,x:(m.x-v.x)*(1-h)+l,y:(m.y-v.y)*(1-h)+u,ratio:a}}},{key:"viewRectangle",value:function(){var r=this.viewportToFramedGraph({x:0,y:0}),a=this.viewportToFramedGraph({x:this.width,y:0}),i=this.viewportToFramedGraph({x:0,y:this.height});return{x1:r.x,y1:r.y,x2:a.x,y2:a.y,height:a.y-i.y}}},{key:"framedGraphToViewport",value:function(r){var a=arguments.length>1&&arguments[1]!==void 0?arguments[1]:{},i=!!a.cameraState||!!a.viewportDimensions||!!a.graphDimensions||!!a.padding,o=a.matrix||(i?Ke(a.cameraState||this.camera.getState(),a.viewportDimensions||this.getDimensions(),a.graphDimensions||this.getGraphDimensions(),a.padding||this.getStagePadding()):this.matrix),s=ht(o,r);return{x:(1+s.x)*this.width/2,y:(1-s.y)*this.height/2}}},{key:"viewportToFramedGraph",value:function(r){var a=arguments.length>1&&arguments[1]!==void 0?arguments[1]:{},i=!!a.cameraState||!!a.viewportDimensions||!!a.graphDimensions||!!a.padding,o=a.matrix||(i?Ke(a.cameraState||this.camera.getState(),a.viewportDimensions||this.getDimensions(),a.graphDimensions||this.getGraphDimensions(),a.padding||this.getStagePadding(),!0):this.invMatrix),s=ht(o,{x:r.x/this.width*2-1,y:1-r.y/this.height*2});return isNaN(s.x)&&(s.x=0),isNaN(s.y)&&(s.y=0),s}},{key:"viewportToGraph",value:function(r){var a=arguments.length>1&&arguments[1]!==void 0?arguments[1]:{};return this.normalizationFunction.inverse(this.viewportToFramedGraph(r,a))}},{key:"graphToViewport",value:function(r){var a=arguments.length>1&&arguments[1]!==void 0?arguments[1]:{};return this.framedGraphToViewport(this.normalizationFunction(r),a)}},{key:"graphToFramedGraph",value:function(r){return this.normalizationFunction(r)}},{key:"framedGraphToGraph",value:function(r){return this.normalizationFunction.inverse(r)}},{key:"getGraphToViewportRatio",value:function(){var r={x:0,y:0},a={x:1,y:1},i=Math.sqrt(Math.pow(r.x-a.x,2)+Math.pow(r.y-a.y,2)),o=this.graphToViewport(r),s=this.graphToViewport(a),l=Math.sqrt(Math.pow(o.x-s.x,2)+Math.pow(o.y-s.y,2));return l/i}},{key:"computeNodeExtent",value:function(){var r=this.nodeGraphCoords,a=Object.keys(r);if(!a.length)return{x:[0,1],y:[0,1]};for(var i=1/0,o=-1/0,s=1/0,l=-1/0,u=0,c=a.length;u<c;u++){var d=r[a[u]],f=d.x,h=d.y;f<i&&(i=f),f>o&&(o=f),h<s&&(s=h),h>l&&(l=h)}return{x:[i,o],y:[s,l]}}},{key:"getBBox",value:function(){return this.nodeExtent}},{key:"getCustomBBox",value:function(){return this.customBBox}},{key:"setCustomBBox",value:function(r){return this.customBBox=r,this.scheduleRender(),this}},{key:"kill",value:function(){var r,a,i,o,s;this.emit("kill"),this.removeAllListeners(),this.camera.cancelAnimation(),this.unbindCameraHandlers(),window.removeEventListener("resize",this.activeListeners.handleResize),this.mouseCaptor.kill(),this.touchCaptor.kill(),this.internals.hoverResolver.kill(),this.unbindGraphHandlers(),this.clearIndices(),this.clearState(),this.internals.nodeDataCache={},this.internals.edgeDataCache={},this.renderFrame&&(cancelAnimationFrame(this.renderFrame),this.renderFrame=null);for(var l=this.container;l.firstChild;)l.removeChild(l.firstChild);this.nodeProgram.kill(),this.nodeFramePass.kill(),this.edgeProgram.kill(),this.edgeFramePass.kill(),this.internals.labelProgram.kill(),(r=this.internals.edgeLabelProgram)===null||r===void 0||r.kill(),(a=this.internals.edgeLabelBackgroundProgram)===null||a===void 0||a.kill(),this.internals.backdropProgram.kill(),this.internals.labelBackgroundProgram.kill(),(i=this.internals.attachmentProgram)===null||i===void 0||i.kill(),(o=this.internals.attachmentManager)===null||o===void 0||o.kill(),this.internals.attachmentProgram=null,this.internals.attachmentManager=null;var u=G(this.customLayerPrograms.values()),c;try{for(u.s();!(c=u.n()).done;){var d=c.value.program;d.kill()}}catch(g){u.e(g)}finally{u.f()}if(this.customLayerPrograms.clear(),this.sdfAtlas&&(this.sdfAtlas=null),this.internals.nodeDataTexture&&(this.internals.nodeDataTexture.kill(),this.internals.nodeDataTexture=null),this.internals.nodeFrameTexture&&(this.internals.nodeFrameTexture.kill(),this.internals.nodeFrameTexture=null),this.internals.edgeDataTexture&&(this.internals.edgeDataTexture.kill(),this.internals.edgeDataTexture=null),this.internals.edgeFrameTexture&&(this.internals.edgeFrameTexture.kill(),this.internals.edgeFrameTexture=null),this.webGLContext){var f;(f=this.webGLContext.getExtension("WEBGL_lose_context"))===null||f===void 0||f.loseContext(),this.webGLContext=null}(s=this.gestureHint)===null||s===void 0||s.kill(),this.gestureHint=null,this.mouseLayer.remove();for(var h in this.extraElements)this.extraElements[h].remove();this.extraElements={}}},{key:"scaleSize",value:function(){var r=arguments.length>0&&arguments[0]!==void 0?arguments[0]:1,a=arguments.length>1&&arguments[1]!==void 0?arguments[1]:this.camera.ratio;return r/this.internals.settings.zoomToSizeRatioFunction(a)*(this.getSetting("itemSizesReference")==="positions"?a*this.graphToViewportRatio:1)}},{key:"getStageCanvas",value:function(){return this.stageCanvas}},{key:"getMouseLayer",value:function(){return this.mouseLayer}}])})(cr),ll=Dg;var ty=Ne(dt());function ul(e){var t=e??{},n=t.segments,r=n===void 0?16:n,a=`
// Compute control point from curvature
// Control point is placed perpendicular to the midpoint of source-target line
vec2 computeControlPoint(vec2 source, vec2 target, float curvature) {
  vec2 midpoint = 0.5 * (source + target);
  vec2 delta = target - source;
  // Perpendicular direction (rotated 90 degrees)
  vec2 perp = vec2(-delta.y, delta.x);
  float len = length(perp);
  if (len < 0.0001) return midpoint;
  perp = perp / len;
  // Offset by curvature * edge length
  return midpoint + perp * curvature * len;
}

// Position at parameter t \u2208 [0, 1]
vec2 path_curved_position(float t, vec2 source, vec2 target) {
  float curvature = v_curvature;
  vec2 control = computeControlPoint(source, target, curvature);
  float u = 1.0 - t;
  return u * u * source + 2.0 * u * t * control + t * t * target;
}

// Derivative of quadratic Bezier (for efficient arc length computation)
vec2 path_curved_derivative(float t, vec2 source, vec2 target) {
  float curvature = v_curvature;
  vec2 control = computeControlPoint(source, target, curvature);
  // B'(t) = 2(1-t)(P1-P0) + 2t(P2-P1)
  return 2.0 * (1.0 - t) * (control - source) + 2.0 * t * (target - control);
}

// Approximate arc length using 5-point Gauss-Legendre quadrature
float path_curved_length(vec2 source, vec2 target) {
  // Gauss-Legendre 5-point weights and abscissae
  const float x1 = 0.9061798459, x2 = 0.5384693101;
  const float w1 = 0.2369268850, w2 = 0.4786286705, w3 = 0.5688888889;

  // Transform from [-1,1] to [0,1]: t = 0.5 * (x + 1)
  float t1a = 0.5 * (-x1 + 1.0), t1b = 0.5 * (x1 + 1.0);
  float t2a = 0.5 * (-x2 + 1.0), t2b = 0.5 * (x2 + 1.0);

  // Evaluate derivative magnitudes at sample points
  float d1a = length(path_curved_derivative(t1a, source, target));
  float d1b = length(path_curved_derivative(t1b, source, target));
  float d2a = length(path_curved_derivative(t2a, source, target));
  float d2b = length(path_curved_derivative(t2b, source, target));
  float d3 = length(path_curved_derivative(0.5, source, target));

  // Sum weighted samples (factor of 0.5 for interval transformation)
  return 0.5 * (w1 * (d1a + d1b) + w2 * (d2a + d2b) + w3 * d3);
}
`;return{name:"curved",segments:r,glsl:a,uniforms:[],attributes:[{name:"curvature",size:1,type:WebGL2RenderingContext.FLOAT}],variables:{curvature:{type:"number",default:0}},spread:{variable:"curvature",compute:function(o,s,l){return(o-(s-1)/2)*l}}}}function cl(e){var t=e??{},n=t.lengthRatio,r=n===void 0?5:n,a=t.widthRatio,i=a===void 0?4:a,o=t.margin,s=o===void 0?0:o,l=`
// Arrow SDF: triangle with base at x=0, tip at x=lengthRatio
// uv.x: 0 (base) to lengthRatio (tip), uv.y: [-halfW, +halfW]
// Returns signed distance (negative inside, positive outside)
float extremity_arrow(vec2 uv, float lengthRatio, float widthRatio) {
  float x = uv.x;
  float y = abs(uv.y);
  float halfW = widthRatio * 0.5;

  // Past the tip: euclidean distance to tip point
  if (x > lengthRatio) {
    return length(vec2(x - lengthRatio, y));
  }

  // Back edge: signed distance to x=0 line
  float backDist = -x;

  // Side edge: signed distance to sloped triangle edge
  float clampedX = max(0.0, x);
  float maxY = halfW * (1.0 - clampedX / lengthRatio);
  float sideDist = y - maxY;

  // Convex shape SDF = max of half-plane distances
  return max(backDist, sideDist);
}
`;return{name:"arrow",glsl:l,length:r,widthFactor:i,margin:s,uniforms:[],attributes:[]}}var iy=Ne(Bt());var ly=Ne(dt());function dl(e,t){return{type:"number",default:e,variable:t?.variable}}function hl(e,t){return{type:"color",default:e,variable:t?.variable}}function fl(e,t){return{type:"boolean",default:e,variable:t?.variable}}function gl(e,t){return{type:{enum:e},default:t,variable:!1}}function pl(e,t){return{type:"array",items:e,minItems:t?.minItems}}function ti(e,t){(t==null||t>e.length)&&(t=e.length);for(var n=0,r=Array(t);n<t;n++)r[n]=e[n];return r}function Cg(e){if(Array.isArray(e))return ti(e)}function kg(e){if(typeof Symbol<"u"&&e[Symbol.iterator]!=null||e["@@iterator"]!=null)return Array.from(e)}function Lg(e,t){if(e){if(typeof e=="string")return ti(e,t);var n={}.toString.call(e).slice(8,-1);return n==="Object"&&e.constructor&&(n=e.constructor.name),n==="Map"||n==="Set"?Array.from(e):n==="Arguments"||/^(?:Ui|I)nt(?:8|16|32)(?:Clamped)?Array$/.test(n)?ti(e,t):void 0}}function Fg(){throw new TypeError(`Invalid attempt to spread non-iterable instance.
In order to be iterable, non-array objects must have a [Symbol.iterator]() method.`)}function Yt(e){return Cg(e)||kg(e)||Lg(e)||Fg()}function Mr(e){"@babel/helpers - typeof";return Mr=typeof Symbol=="function"&&typeof Symbol.iterator=="symbol"?function(t){return typeof t}:function(t){return t&&typeof Symbol=="function"&&t.constructor===Symbol&&t!==Symbol.prototype?"symbol":typeof t},Mr(e)}function ni(e){return Mr(e)==="object"&&e!==null&&"attribute"in e}function vl(e){return typeof e=="string"}function zr(e){return Mr(e)==="object"&&e!==null&&"attribute"in e}function Pg(e){var t=e.filter(function(v){return v.fill}).length,n=re(t||1),r=e.flatMap(function(v,p){if(v.fill)return[];var b=v.size,y=v.mode,_=y==="pixels",x;return zr(b)?x="v_borderSize_".concat(p+1):x=re(typeof b=="number"?b:0),_?["  float borderSize_".concat(p+1," = ").concat(x," * context.pixelToUV;")]:["  float borderSize_".concat(p+1," = context.shapeHalfSize * ").concat(x,";")]}).join(`
`),a=e.flatMap(function(v,p){return v.fill?[]:["borderSize_".concat(p+1)]}).join(" + "),i=a||"0.0",o=e.flatMap(function(v,p){return v.fill?["  float borderSize_".concat(p+1," = fillBorderSize;")]:[]}).join(`
`),s=e.map(function(v,p){return"  float boundary_".concat(p+1," = boundary_").concat(p," - borderSize_").concat(p+1,";")}).join(`
`),l=e.map(function(v,p){var b=v.color;return ni(b)?"  vec4 borderColor_".concat(p+1," = v_borderColor_").concat(p+1,";"):"  vec4 borderColor_".concat(p+1," = u_borderColor_").concat(p+1,";")}).join(`
`),u=e.map(function(v,p){return"  borderColor_".concat(p+1,`.a *= bias;
  if (borderSize_`).concat(p+1," <= 2.0 * context.aaWidth) { borderColor_").concat(p+1," = ").concat(p===0?"borderColor_1":"borderColor_".concat(p),"; }")}).join(`
`),c=e.map(function(v,p){return p===0?`if (context.sdf > boundary_1) {
    color = borderColor_1;
  } else `:"if (context.sdf > boundary_".concat(p,` - 2.0 * context.aaWidth) {
    color = mix(borderColor_`).concat(p+1,", borderColor_").concat(p,", (context.sdf - boundary_").concat(p,` + 2.0 * context.aaWidth) / (2.0 * context.aaWidth));
  } else if (context.sdf > boundary_`).concat(p+1,`) {
    color = borderColor_`).concat(p+1,`;
  } else `)}).join(""),d=[].concat(Yt(e.flatMap(function(v,p){var b=v.color;return ni(b)?["vec4 v_borderColor_".concat(p+1)]:[]})),Yt(e.flatMap(function(v,p){var b=v.size;return zr(b)?["float v_borderSize_".concat(p+1)]:[]}))),f=e.flatMap(function(v,p){var b=v.color;return vl(b)?["vec4 u_borderColor_".concat(p+1)]:[]}),h=[].concat(Yt(d),Yt(f)).join(", "),g=zr(e[0].size),m=`
vec4 layer_border(`.concat(h,`) {
  const float bias = 255.0 / 254.0;

  // Calculate border sizes (using context.shapeSize and context.pixelSize)
`).concat(r,`
`).concat(g?`
  // Early return if first border size is effectively zero (layer disabled)
  if (borderSize_1 <= context.aaWidth) {
    return vec4(0.0);
  }
`:"",`
  // Calculate fill border size (distribute remaining space)
  // Use inradiusFactor to get actual shape depth from the bounding size
  // For circle/square (inradiusFactor=1.0), this equals shapeSize
  // For triangle (inradiusFactor=0.5), this is half of shapeSize
  float shapeDepth = context.shapeSize * context.inradiusFactor;
  float fillBorderSize = (shapeDepth - (`).concat(i,")) / ").concat(n,`;
`).concat(o,`

  // Calculate cumulative boundaries (from outside to inside in SDF space)
  float boundary_0 = 0.0;  // Shape edge at context.sdf=0
`).concat(s,`

  // Set up colors
`).concat(l,`
`).concat(u,`

  // Select color based on SDF position with antialiasing
  // Note: outer edge AA (context.sdf > 0 transition) is handled by the composed generator's smoothstep
  vec4 color = vec4(0.0);
  `).concat(c,"{ color = borderColor_").concat(e.length,`; }

  return color;
}
`);return m}function ml(e){var t,n=(t=e?.borders)!==null&&t!==void 0?t:[];if(n.length===0)return{name:"border",uniforms:[],attributes:[],glsl:"vec4 layer_border() { return vec4(0.0); }"};var r=WebGL2RenderingContext,a=r.UNSIGNED_BYTE,i=r.FLOAT,o=n.flatMap(function(l,u){var c=l.color;return vl(c)?[{name:"u_borderColor_".concat(u+1),type:"vec4",value:un(c)}]:[]}),s=[].concat(Yt(n.flatMap(function(l,u){var c=l.color;return ni(c)?[{name:"borderColor_".concat(u+1),size:4,type:a,normalized:!0,source:c.attribute,defaultValue:c.default}]:[]})),Yt(n.flatMap(function(l,u){var c=l.size;return zr(c)?[{name:"borderSize_".concat(u+1),size:1,type:i,source:c.attribute,defaultValue:c.default}]:[]})));return{name:"border",uniforms:o,attributes:s,glsl:Pg(n)}}var fy={borders:pl({size:dl(.1,{variable:!0}),color:hl("#000000",{variable:!0}),mode:gl(["relative","pixels"],"relative"),fill:fl(!1)})};var cc=Ne(jl(),1);function np(e){let t=+this._x.call(null,e),n=+this._y.call(null,e);return Vl(this.cover(t,n),t,n,e)}function Vl(e,t,n,r){if(isNaN(t)||isNaN(n))return e;var a,i=e._root,o={data:r},s=e._x0,l=e._y0,u=e._x1,c=e._y1,d,f,h,g,m,v,p,b;if(!i)return e._root=o,e;for(;i.length;)if((m=t>=(d=(s+u)/2))?s=d:u=d,(v=n>=(f=(l+c)/2))?l=f:c=f,a=i,!(i=i[p=v<<1|m]))return a[p]=o,e;if(h=+e._x.call(null,i.data),g=+e._y.call(null,i.data),t===h&&n===g)return o.next=i,a?a[p]=o:e._root=o,e;do a=a?a[p]=new Array(4):e._root=new Array(4),(m=t>=(d=(s+u)/2))?s=d:u=d,(v=n>=(f=(l+c)/2))?l=f:c=f;while((p=v<<1|m)===(b=(g>=f)<<1|h>=d));return a[b]=i,a[p]=o,e}function rp(e){Array.isArray(e)||(e=Array.from(e));let t=e.length,n=new Float64Array(t),r=new Float64Array(t),a=1/0,i=a,o=-a,s=o;for(let l=0,u,c,d;l<t;++l){if(isNaN(c=+this._x.call(null,u=e[l]))||isNaN(d=+this._y.call(null,u))){n[l]=NaN;continue}n[l]=c,r[l]=d,c<a&&(a=c),c>o&&(o=c),d<i&&(i=d),d>s&&(s=d)}if(a>o||i>s)return this;this.cover(a,i).cover(o,s);for(let l=0;l<t;++l)Vl(this,n[l],r[l],e[l]);return this}function ap(e,t){if(isNaN(e=+e)||isNaN(t=+t))return this;var n=this._x0,r=this._y0,a=this._x1,i=this._y1;if(isNaN(n))a=(n=Math.floor(e))+1,i=(r=Math.floor(t))+1;else{for(var o=a-n||1,s=this._root,l,u;n>e||e>=a||r>t||t>=i;)switch(u=(t<r)<<1|e<n,l=new Array(4),l[u]=s,s=l,o*=2,u){case 0:a=n+o,i=r+o;break;case 1:n=a-o,i=r+o;break;case 2:a=n+o,r=i-o;break;case 3:n=a-o,r=i-o;break}this._root&&this._root.length&&(this._root=s)}return this._x0=n,this._y0=r,this._x1=a,this._y1=i,this}function ip(){var e=[];return this.visit(function(t){if(!t.length)do e.push(t.data);while(t=t.next)}),e}function op(e){return arguments.length?this.cover(+e[0][0],+e[0][1]).cover(+e[1][0],+e[1][1]):isNaN(this._x0)?void 0:[[this._x0,this._y0],[this._x1,this._y1]]}function ve(e,t,n,r,a){this.node=e,this.x0=t,this.y0=n,this.x1=r,this.y1=a}function sp(e,t,n){var r,a=this._x0,i=this._y0,o,s,l,u,c=this._x1,d=this._y1,f=[],h=this._root,g,m;for(h&&f.push(new ve(h,a,i,c,d)),n==null?n=1/0:(a=e-n,i=t-n,c=e+n,d=t+n,n*=n);g=f.pop();)if(!(!(h=g.node)||(o=g.x0)>c||(s=g.y0)>d||(l=g.x1)<a||(u=g.y1)<i))if(h.length){var v=(o+l)/2,p=(s+u)/2;f.push(new ve(h[3],v,p,l,u),new ve(h[2],o,p,v,u),new ve(h[1],v,s,l,p),new ve(h[0],o,s,v,p)),(m=(t>=p)<<1|e>=v)&&(g=f[f.length-1],f[f.length-1]=f[f.length-1-m],f[f.length-1-m]=g)}else{var b=e-+this._x.call(null,h.data),y=t-+this._y.call(null,h.data),_=b*b+y*y;if(_<n){var x=Math.sqrt(n=_);a=e-x,i=t-x,c=e+x,d=t+x,r=h.data}}return r}function lp(e){if(isNaN(c=+this._x.call(null,e))||isNaN(d=+this._y.call(null,e)))return this;var t,n=this._root,r,a,i,o=this._x0,s=this._y0,l=this._x1,u=this._y1,c,d,f,h,g,m,v,p;if(!n)return this;if(n.length)for(;;){if((g=c>=(f=(o+l)/2))?o=f:l=f,(m=d>=(h=(s+u)/2))?s=h:u=h,t=n,!(n=n[v=m<<1|g]))return this;if(!n.length)break;(t[v+1&3]||t[v+2&3]||t[v+3&3])&&(r=t,p=v)}for(;n.data!==e;)if(a=n,!(n=n.next))return this;return(i=n.next)&&delete n.next,a?(i?a.next=i:delete a.next,this):t?(i?t[v]=i:delete t[v],(n=t[0]||t[1]||t[2]||t[3])&&n===(t[3]||t[2]||t[1]||t[0])&&!n.length&&(r?r[p]=n:this._root=n),this):(this._root=i,this)}function up(e){for(var t=0,n=e.length;t<n;++t)this.remove(e[t]);return this}function cp(){return this._root}function dp(){var e=0;return this.visit(function(t){if(!t.length)do++e;while(t=t.next)}),e}function hp(e){var t=[],n,r=this._root,a,i,o,s,l;for(r&&t.push(new ve(r,this._x0,this._y0,this._x1,this._y1));n=t.pop();)if(!e(r=n.node,i=n.x0,o=n.y0,s=n.x1,l=n.y1)&&r.length){var u=(i+s)/2,c=(o+l)/2;(a=r[3])&&t.push(new ve(a,u,c,s,l)),(a=r[2])&&t.push(new ve(a,i,c,u,l)),(a=r[1])&&t.push(new ve(a,u,o,s,c)),(a=r[0])&&t.push(new ve(a,i,o,u,c))}return this}function fp(e){var t=[],n=[],r;for(this._root&&t.push(new ve(this._root,this._x0,this._y0,this._x1,this._y1));r=t.pop();){var a=r.node;if(a.length){var i,o=r.x0,s=r.y0,l=r.x1,u=r.y1,c=(o+l)/2,d=(s+u)/2;(i=a[0])&&t.push(new ve(i,o,s,c,d)),(i=a[1])&&t.push(new ve(i,c,s,l,d)),(i=a[2])&&t.push(new ve(i,o,d,c,u)),(i=a[3])&&t.push(new ve(i,c,d,l,u))}n.push(r)}for(;r=n.pop();)e(r.node,r.x0,r.y0,r.x1,r.y1);return this}function gp(e){return e[0]}function pp(e){return arguments.length?(this._x=e,this):this._x}function vp(e){return e[1]}function mp(e){return arguments.length?(this._y=e,this):this._y}function Hr(e,t,n){var r=new li(t??gp,n??vp,NaN,NaN,NaN,NaN);return e==null?r:r.addAll(e)}function li(e,t,n,r,a,i){this._x=e,this._y=t,this._x0=n,this._y0=r,this._x1=a,this._y1=i,this._root=void 0}function $l(e){for(var t={data:e.data},n=t;e=e.next;)n=n.next={data:e.data};return t}var me=Hr.prototype=li.prototype;me.copy=function(){var e=new li(this._x,this._y,this._x0,this._y0,this._x1,this._y1),t=this._root,n,r;if(!t)return e;if(!t.length)return e._root=$l(t),e;for(n=[{source:t,target:e._root=new Array(4)}];t=n.pop();)for(var a=0;a<4;++a)(r=t.source[a])&&(r.length?n.push({source:r,target:t.target[a]=new Array(4)}):t.target[a]=$l(r));return e};me.add=np;me.addAll=rp;me.cover=ap;me.data=ip;me.extent=op;me.find=sp;me.remove=lp;me.removeAll=up;me.root=cp;me.size=dp;me.visit=hp;me.visitAfter=fp;me.x=pp;me.y=mp;var bp={value:()=>{}};function ui(){for(var e=0,t=arguments.length,n={},r;e<t;++e){if(!(r=arguments[e]+"")||r in n||/[\s.]/.test(r))throw new Error("illegal type: "+r);n[r]=[]}return new jr(n)}function jr(e){this._=e}function yp(e,t){return e.trim().split(/^|\s+/).map(function(n){var r="",a=n.indexOf(".");if(a>=0&&(r=n.slice(a+1),n=n.slice(0,a)),n&&!t.hasOwnProperty(n))throw new Error("unknown type: "+n);return{type:n,name:r}})}jr.prototype=ui.prototype={constructor:jr,on:function(e,t){var n=this._,r=yp(e+"",n),a,i=-1,o=r.length;if(arguments.length<2){for(;++i<o;)if((a=(e=r[i]).type)&&(a=_p(n[a],e.name)))return a;return}if(t!=null&&typeof t!="function")throw new Error("invalid callback: "+t);for(;++i<o;)if(a=(e=r[i]).type)n[a]=ql(n[a],e.name,t);else if(t==null)for(a in n)n[a]=ql(n[a],e.name,null);return this},copy:function(){var e={},t=this._;for(var n in t)e[n]=t[n].slice();return new jr(e)},call:function(e,t){if((a=arguments.length-2)>0)for(var n=new Array(a),r=0,a,i;r<a;++r)n[r]=arguments[r+2];if(!this._.hasOwnProperty(e))throw new Error("unknown type: "+e);for(i=this._[e],r=0,a=i.length;r<a;++r)i[r].value.apply(t,n)},apply:function(e,t,n){if(!this._.hasOwnProperty(e))throw new Error("unknown type: "+e);for(var r=this._[e],a=0,i=r.length;a<i;++a)r[a].value.apply(t,n)}};function _p(e,t){for(var n=0,r=e.length,a;n<r;++n)if((a=e[n]).name===t)return a.value}function ql(e,t,n){for(var r=0,a=e.length;r<a;++r)if(e[r].name===t){e[r]=bp,e=e.slice(0,r).concat(e.slice(r+1));break}return n!=null&&e.push({name:t,value:n}),e}var Kt=0,Dn=0,wn=0,Yl=1e3,$r,Cn,Vr=0,St=0,qr=0,kn=typeof performance=="object"&&performance.now?performance:Date,Kl=typeof window=="object"&&window.requestAnimationFrame?window.requestAnimationFrame.bind(window):function(e){setTimeout(e,17)};function Zl(){return St||(Kl(xp),St=kn.now()+qr)}function xp(){St=0}function ci(){this._call=this._time=this._next=null}ci.prototype=hi.prototype={constructor:ci,restart:function(e,t,n){if(typeof e!="function")throw new TypeError("callback is not a function");n=(n==null?Zl():+n)+(t==null?0:+t),!this._next&&Cn!==this&&(Cn?Cn._next=this:$r=this,Cn=this),this._call=e,this._time=n,di()},stop:function(){this._call&&(this._call=null,this._time=1/0,di())}};function hi(e,t,n){var r=new ci;return r.restart(e,t,n),r}function Tp(){Zl(),++Kt;for(var e=$r,t;e;)(t=St-e._time)>=0&&e._call.call(void 0,t),e=e._next;--Kt}function Xl(){St=(Vr=kn.now())+qr,Kt=Dn=0;try{Tp()}finally{Kt=0,Sp(),St=0}}function Ep(){var e=kn.now(),t=e-Vr;t>Yl&&(qr-=t,Vr=e)}function Sp(){for(var e,t=$r,n,r=1/0;t;)t._call?(r>t._time&&(r=t._time),e=t,t=t._next):(n=t._next,t._next=null,t=e?e._next=n:$r=n);Cn=e,di(r)}function di(e){if(!Kt){Dn&&(Dn=clearTimeout(Dn));var t=e-St;t>24?(e<1/0&&(Dn=setTimeout(Xl,Math.ceil(e-kn.now()-qr)+1)),wn&&(wn=clearInterval(wn))):(wn||(Vr=kn.now(),wn=setInterval(Ep,Yl)),Kt=1,Kl(Xl))}}function be(e){return function(){return e}}function ot(e){return(e()-.5)*1e-6}function Rp(e){return e.x+e.vx}function Ap(e){return e.y+e.vy}function eu(e){var t,n,r,a=1,i=1;typeof e!="function"&&(e=be(e==null?1:+e));function o(){for(var u,c=t.length,d,f,h,g,m,v,p=0;p<i;++p)for(d=Hr(t,Rp,Ap).visitAfter(s),u=0;u<c;++u)f=t[u],m=n[f.index],v=m*m,h=f.x+f.vx,g=f.y+f.vy,d.visit(b);function b(y,_,x,T,E){var S=y.data,w=y.r,A=m+w;if(S){if(S.index>f.index){var C=h-S.x-S.vx,D=g-S.y-S.vy,F=C*C+D*D;F<A*A&&(C===0&&(C=ot(r),F+=C*C),D===0&&(D=ot(r),F+=D*D),F=(A-(F=Math.sqrt(F)))/F*a,f.vx+=(C*=F)*(A=(w*=w)/(v+w)),f.vy+=(D*=F)*A,S.vx-=C*(A=1-A),S.vy-=D*A)}return}return _>h+A||T<h-A||x>g+A||E<g-A}}function s(u){if(u.data)return u.r=n[u.data.index];for(var c=u.r=0;c<4;++c)u[c]&&u[c].r>u.r&&(u.r=u[c].r)}function l(){if(t){var u,c=t.length,d;for(n=new Array(c),u=0;u<c;++u)d=t[u],n[d.index]=+e(d,u,t)}}return o.initialize=function(u,c){t=u,r=c,l()},o.iterations=function(u){return arguments.length?(i=+u,o):i},o.strength=function(u){return arguments.length?(a=+u,o):a},o.radius=function(u){return arguments.length?(e=typeof u=="function"?u:be(+u),l(),o):e},o}function wp(e){return e.index}function Jl(e,t){var n=e.get(t);if(!n)throw new Error("node not found: "+t);return n}function tu(e){var t=wp,n=d,r,a=be(30),i,o,s,l,u,c=1;e==null&&(e=[]);function d(v){return 1/Math.min(s[v.source.index],s[v.target.index])}function f(v){for(var p=0,b=e.length;p<c;++p)for(var y=0,_,x,T,E,S,w,A;y<b;++y)_=e[y],x=_.source,T=_.target,E=T.x+T.vx-x.x-x.vx||ot(u),S=T.y+T.vy-x.y-x.vy||ot(u),w=Math.sqrt(E*E+S*S),w=(w-i[y])/w*v*r[y],E*=w,S*=w,T.vx-=E*(A=l[y]),T.vy-=S*A,x.vx+=E*(A=1-A),x.vy+=S*A}function h(){if(o){var v,p=o.length,b=e.length,y=new Map(o.map((x,T)=>[t(x,T,o),x])),_;for(v=0,s=new Array(p);v<b;++v)_=e[v],_.index=v,typeof _.source!="object"&&(_.source=Jl(y,_.source)),typeof _.target!="object"&&(_.target=Jl(y,_.target)),s[_.source.index]=(s[_.source.index]||0)+1,s[_.target.index]=(s[_.target.index]||0)+1;for(v=0,l=new Array(b);v<b;++v)_=e[v],l[v]=s[_.source.index]/(s[_.source.index]+s[_.target.index]);r=new Array(b),g(),i=new Array(b),m()}}function g(){if(o)for(var v=0,p=e.length;v<p;++v)r[v]=+n(e[v],v,e)}function m(){if(o)for(var v=0,p=e.length;v<p;++v)i[v]=+a(e[v],v,e)}return f.initialize=function(v,p){o=v,u=p,h()},f.links=function(v){return arguments.length?(e=v,h(),f):e},f.id=function(v){return arguments.length?(t=v,f):t},f.iterations=function(v){return arguments.length?(c=+v,f):c},f.strength=function(v){return arguments.length?(n=typeof v=="function"?v:be(+v),g(),f):n},f.distance=function(v){return arguments.length?(a=typeof v=="function"?v:be(+v),m(),f):a},f}var Dp=1664525,Cp=1013904223,Ql=4294967296;function kp(){let e=1;return()=>(e=(Dp*e+Cp)%Ql)/Ql}function Lp(e){return e.x}function Fp(e){return e.y}var Pp=10,Ip=Math.PI*(3-Math.sqrt(5));function nu(e){var t,n=1,r=.001,a=1-Math.pow(r,1/300),i=0,o=.6,s=new Map,l=hi(d),u=ui("tick","end"),c=kp();e==null&&(e=[]);function d(){f(),u.call("tick",t),n<r&&(l.stop(),u.call("end",t))}function f(m){var v,p=e.length,b;m===void 0&&(m=1);for(var y=0;y<m;++y)for(n+=(i-n)*a,s.forEach(function(_){_(n)}),v=0;v<p;++v)b=e[v],b.fx==null?b.x+=b.vx*=o:(b.x=b.fx,b.vx=0),b.fy==null?b.y+=b.vy*=o:(b.y=b.fy,b.vy=0);return t}function h(){for(var m=0,v=e.length,p;m<v;++m){if(p=e[m],p.index=m,p.fx!=null&&(p.x=p.fx),p.fy!=null&&(p.y=p.fy),isNaN(p.x)||isNaN(p.y)){var b=Pp*Math.sqrt(.5+m),y=m*Ip;p.x=b*Math.cos(y),p.y=b*Math.sin(y)}(isNaN(p.vx)||isNaN(p.vy))&&(p.vx=p.vy=0)}}function g(m){return m.initialize&&m.initialize(e,c),m}return h(),t={tick:f,restart:function(){return l.restart(d),t},stop:function(){return l.stop(),t},nodes:function(m){return arguments.length?(e=m,h(),s.forEach(g),t):e},alpha:function(m){return arguments.length?(n=+m,t):n},alphaMin:function(m){return arguments.length?(r=+m,t):r},alphaDecay:function(m){return arguments.length?(a=+m,t):+a},alphaTarget:function(m){return arguments.length?(i=+m,t):i},velocityDecay:function(m){return arguments.length?(o=1-m,t):1-o},randomSource:function(m){return arguments.length?(c=m,s.forEach(g),t):c},force:function(m,v){return arguments.length>1?(v==null?s.delete(m):s.set(m,g(v)),t):s.get(m)},find:function(m,v,p){var b=0,y=e.length,_,x,T,E,S;for(p==null?p=1/0:p*=p,b=0;b<y;++b)E=e[b],_=m-E.x,x=v-E.y,T=_*_+x*x,T<p&&(S=E,p=T);return S},on:function(m,v){return arguments.length>1?(u.on(m,v),t):u.on(m)}}}function ru(){var e,t,n,r,a=be(-30),i,o=1,s=1/0,l=.81;function u(h){var g,m=e.length,v=Hr(e,Lp,Fp).visitAfter(d);for(r=h,g=0;g<m;++g)t=e[g],v.visit(f)}function c(){if(e){var h,g=e.length,m;for(i=new Array(g),h=0;h<g;++h)m=e[h],i[m.index]=+a(m,h,e)}}function d(h){var g=0,m,v,p=0,b,y,_;if(h.length){for(b=y=_=0;_<4;++_)(m=h[_])&&(v=Math.abs(m.value))&&(g+=m.value,p+=v,b+=v*m.x,y+=v*m.y);h.x=b/p,h.y=y/p}else{m=h,m.x=m.data.x,m.y=m.data.y;do g+=i[m.data.index];while(m=m.next)}h.value=g}function f(h,g,m,v){if(!h.value)return!0;var p=h.x-t.x,b=h.y-t.y,y=v-g,_=p*p+b*b;if(y*y/l<_)return _<s&&(p===0&&(p=ot(n),_+=p*p),b===0&&(b=ot(n),_+=b*b),_<o&&(_=Math.sqrt(o*_)),t.vx+=p*h.value*r/_,t.vy+=b*h.value*r/_),!0;if(h.length||_>=s)return;(h.data!==t||h.next)&&(p===0&&(p=ot(n),_+=p*p),b===0&&(b=ot(n),_+=b*b),_<o&&(_=Math.sqrt(o*_)));do h.data!==t&&(y=i[h.data.index]*r/_,t.vx+=p*y,t.vy+=b*y);while(h=h.next)}return u.initialize=function(h,g){e=h,n=g,c()},u.strength=function(h){return arguments.length?(a=typeof h=="function"?h:be(+h),c(),u):a},u.distanceMin=function(h){return arguments.length?(o=h*h,u):Math.sqrt(o)},u.distanceMax=function(h){return arguments.length?(s=h*h,u):Math.sqrt(s)},u.theta=function(h){return arguments.length?(l=h*h,u):Math.sqrt(l)},u}function au(e){var t=be(.1),n,r,a;typeof e!="function"&&(e=be(e==null?0:+e));function i(s){for(var l=0,u=n.length,c;l<u;++l)c=n[l],c.vx+=(a[l]-c.x)*r[l]*s}function o(){if(n){var s,l=n.length;for(r=new Array(l),a=new Array(l),s=0;s<l;++s)r[s]=isNaN(a[s]=+e(n[s],s,n))?0:+t(n[s],s,n)}}return i.initialize=function(s){n=s,o()},i.strength=function(s){return arguments.length?(t=typeof s=="function"?s:be(+s),o(),i):t},i.x=function(s){return arguments.length?(e=typeof s=="function"?s:be(+s),o(),i):e},i}function iu(e){var t=be(.1),n,r,a;typeof e!="function"&&(e=be(e==null?0:+e));function i(s){for(var l=0,u=n.length,c;l<u;++l)c=n[l],c.vy+=(a[l]-c.y)*r[l]*s}function o(){if(n){var s,l=n.length;for(r=new Array(l),a=new Array(l),s=0;s<l;++s)r[s]=isNaN(a[s]=+e(n[s],s,n))?0:+t(n[s],s,n)}}return i.initialize=function(s){n=s,o()},i.strength=function(s){return arguments.length?(t=typeof s=="function"?s:be(+s),o(),i):t},i.y=function(s){return arguments.length?(e=typeof s=="function"?s:be(+s),o(),i):e},i}var fi={};Pi(fi,{decode:()=>Ln,encode:()=>st,format:()=>Zt,parse:()=>Fn});var ou={};function Np(e){let t=ou[e];if(t)return t;t=ou[e]=[];for(let n=0;n<128;n++){let r=String.fromCharCode(n);t.push(r)}for(let n=0;n<e.length;n++){let r=e.charCodeAt(n);t[r]="%"+("0"+r.toString(16).toUpperCase()).slice(-2)}return t}function Xr(e,t){typeof t!="string"&&(t=Xr.defaultChars);let n=Np(t);return e.replace(/(%[a-f0-9]{2})+/gi,function(r){let a="";for(let i=0,o=r.length;i<o;i+=3){let s=parseInt(r.slice(i+1,i+3),16);if(s<128){a+=n[s];continue}if((s&224)===192&&i+3<o){let l=parseInt(r.slice(i+4,i+6),16);if((l&192)===128){let u=s<<6&1984|l&63;u<128?a+="\uFFFD\uFFFD":a+=String.fromCharCode(u),i+=3;continue}}if((s&240)===224&&i+6<o){let l=parseInt(r.slice(i+4,i+6),16),u=parseInt(r.slice(i+7,i+9),16);if((l&192)===128&&(u&192)===128){let c=s<<12&61440|l<<6&4032|u&63;c<2048||c>=55296&&c<=57343?a+="\uFFFD\uFFFD\uFFFD":a+=String.fromCharCode(c),i+=6;continue}}if((s&248)===240&&i+9<o){let l=parseInt(r.slice(i+4,i+6),16),u=parseInt(r.slice(i+7,i+9),16),c=parseInt(r.slice(i+10,i+12),16);if((l&192)===128&&(u&192)===128&&(c&192)===128){let d=s<<18&1835008|l<<12&258048|u<<6&4032|c&63;d<65536||d>1114111?a+="\uFFFD\uFFFD\uFFFD\uFFFD":(d-=65536,a+=String.fromCharCode(55296+(d>>10),56320+(d&1023))),i+=9;continue}}a+="\uFFFD"}return a})}Xr.defaultChars=";/?:@&=+$,#";Xr.componentChars="";var Ln=Xr;var su={};function zp(e){let t=su[e];if(t)return t;t=su[e]=[];for(let n=0;n<128;n++){let r=String.fromCharCode(n);/^[0-9a-z]$/i.test(r)?t.push(r):t.push("%"+("0"+n.toString(16).toUpperCase()).slice(-2))}for(let n=0;n<e.length;n++)t[e.charCodeAt(n)]=e[n];return t}function Yr(e,t,n){typeof t!="string"&&(n=t,t=Yr.defaultChars),typeof n>"u"&&(n=!0);let r=zp(t),a="";for(let i=0,o=e.length;i<o;i++){let s=e.charCodeAt(i);if(n&&s===37&&i+2<o&&/^[0-9a-f]{2}$/i.test(e.slice(i+1,i+3))){a+=e.slice(i,i+3),i+=2;continue}if(s<128){a+=r[s];continue}if(s>=55296&&s<=57343){if(s>=55296&&s<=56319&&i+1<o){let l=e.charCodeAt(i+1);if(l>=56320&&l<=57343){a+=encodeURIComponent(e[i]+e[i+1]),i++;continue}}a+="%EF%BF%BD";continue}a+=encodeURIComponent(e[i])}return a}Yr.defaultChars=";/?:@&=+$,-_.!~*'()#";Yr.componentChars="-_.!~*'()";var st=Yr;function Zt(e){let t="";return t+=e.protocol||"",t+=e.slashes?"//":"",t+=e.auth?e.auth+"@":"",e.hostname&&e.hostname.indexOf(":")!==-1?t+="["+e.hostname+"]":t+=e.hostname||"",t+=e.port?":"+e.port:"",t+=e.pathname||"",t+=e.search||"",t+=e.hash||"",t}function Kr(){this.protocol=null,this.slashes=null,this.auth=null,this.port=null,this.hostname=null,this.hash=null,this.search=null,this.pathname=null}var Mp=/^([a-z0-9.+-]+:)/i,Gp=/:[0-9]*$/,Bp=/^(\/\/?(?!\/)[^\?\s]*)(\?[^\s]*)?$/,Wp=["<",">",'"',"`"," ","\r",`
`,"	"],Op=["{","}","|","\\","^","`"].concat(Wp),Up=["'"].concat(Op),lu=["%","/","?",";","#"].concat(Up),uu=["/","?","#"],Hp=255,cu=/^[+a-z0-9A-Z_-]{0,63}$/,jp=/^([+a-z0-9A-Z_-]{0,63})(.*)$/,du={javascript:!0,"javascript:":!0},hu={http:!0,https:!0,ftp:!0,gopher:!0,file:!0,"http:":!0,"https:":!0,"ftp:":!0,"gopher:":!0,"file:":!0};function $p(e,t){if(e&&e instanceof Kr)return e;let n=new Kr;return n.parse(e,t),n}Kr.prototype.parse=function(e,t){let n,r,a,i=e;if(i=i.trim(),!t&&e.split("#").length===1){let u=Bp.exec(i);if(u)return this.pathname=u[1],u[2]&&(this.search=u[2]),this}let o=Mp.exec(i);if(o&&(o=o[0],n=o.toLowerCase(),this.protocol=o,i=i.substr(o.length)),(t||o||i.match(/^\/\/[^@\/]+@[^@\/]+/))&&(a=i.substr(0,2)==="//",a&&!(o&&du[o])&&(i=i.substr(2),this.slashes=!0)),!du[o]&&(a||o&&!hu[o])){let u=-1;for(let g=0;g<uu.length;g++)r=i.indexOf(uu[g]),r!==-1&&(u===-1||r<u)&&(u=r);let c,d;u===-1?d=i.lastIndexOf("@"):d=i.lastIndexOf("@",u),d!==-1&&(c=i.slice(0,d),i=i.slice(d+1),this.auth=c),u=-1;for(let g=0;g<lu.length;g++)r=i.indexOf(lu[g]),r!==-1&&(u===-1||r<u)&&(u=r);u===-1&&(u=i.length),i[u-1]===":"&&u--;let f=i.slice(0,u);i=i.slice(u),this.parseHost(f),this.hostname=this.hostname||"";let h=this.hostname[0]==="["&&this.hostname[this.hostname.length-1]==="]";if(!h){let g=this.hostname.split(/\./);for(let m=0,v=g.length;m<v;m++){let p=g[m];if(p&&!p.match(cu)){let b="";for(let y=0,_=p.length;y<_;y++)p.charCodeAt(y)>127?b+="x":b+=p[y];if(!b.match(cu)){let y=g.slice(0,m),_=g.slice(m+1),x=p.match(jp);x&&(y.push(x[1]),_.unshift(x[2])),_.length&&(i=_.join(".")+i),this.hostname=y.join(".");break}}}}this.hostname.length>Hp&&(this.hostname=""),h&&(this.hostname=this.hostname.substr(1,this.hostname.length-2))}let s=i.indexOf("#");s!==-1&&(this.hash=i.substr(s),i=i.slice(0,s));let l=i.indexOf("?");return l!==-1&&(this.search=i.substr(l),i=i.slice(0,l)),i&&(this.pathname=i),hu[n]&&this.hostname&&!this.pathname&&(this.pathname=""),this};Kr.prototype.parseHost=function(e){let t=Gp.exec(e);t&&(t=t[0],t!==":"&&(this.port=t.substr(1)),e=e.substr(0,e.length-t.length)),e&&(this.hostname=e)};var Fn=$p;var bi={};Pi(bi,{Any:()=>gi,Cc:()=>pi,Cf:()=>Vp,P:()=>Pn,S:()=>vi,Z:()=>mi});var gi=/[\0-\uD7FF\uE000-\uFFFF]|[\uD800-\uDBFF][\uDC00-\uDFFF]|[\uD800-\uDBFF](?![\uDC00-\uDFFF])|(?:[^\uD800-\uDBFF]|^)[\uDC00-\uDFFF]/,pi=/[\0-\x1F\x7F-\x9F]/,Vp=/[\xAD\u0600-\u0605\u061C\u06DD\u070F\u0890\u0891\u08E2\u180E\u200B-\u200F\u202A-\u202E\u2060-\u2064\u2066-\u206F\uFEFF\uFFF9-\uFFFB]|\uD804[\uDCBD\uDCCD]|\uD80D[\uDC30-\uDC3F]|\uD82F[\uDCA0-\uDCA3]|\uD834[\uDD73-\uDD7A]|\uDB40[\uDC01\uDC20-\uDC7F]/,Pn=/[!-#%-\*,-\/:;\?@\[-\]_\{\}\xA1\xA7\xAB\xB6\xB7\xBB\xBF\u037E\u0387\u055A-\u055F\u0589\u058A\u05BE\u05C0\u05C3\u05C6\u05F3\u05F4\u0609\u060A\u060C\u060D\u061B\u061D-\u061F\u066A-\u066D\u06D4\u0700-\u070D\u07F7-\u07F9\u0830-\u083E\u085E\u0964\u0965\u0970\u09FD\u0A76\u0AF0\u0C77\u0C84\u0DF4\u0E4F\u0E5A\u0E5B\u0F04-\u0F12\u0F14\u0F3A-\u0F3D\u0F85\u0FD0-\u0FD4\u0FD9\u0FDA\u104A-\u104F\u10FB\u1360-\u1368\u1400\u166E\u169B\u169C\u16EB-\u16ED\u1735\u1736\u17D4-\u17D6\u17D8-\u17DA\u1800-\u180A\u1944\u1945\u1A1E\u1A1F\u1AA0-\u1AA6\u1AA8-\u1AAD\u1B4E\u1B4F\u1B5A-\u1B60\u1B7D-\u1B7F\u1BFC-\u1BFF\u1C3B-\u1C3F\u1C7E\u1C7F\u1CC0-\u1CC7\u1CD3\u2010-\u2027\u2030-\u2043\u2045-\u2051\u2053-\u205E\u207D\u207E\u208D\u208E\u2308-\u230B\u2329\u232A\u2768-\u2775\u27C5\u27C6\u27E6-\u27EF\u2983-\u2998\u29D8-\u29DB\u29FC\u29FD\u2CF9-\u2CFC\u2CFE\u2CFF\u2D70\u2E00-\u2E2E\u2E30-\u2E4F\u2E52-\u2E5D\u3001-\u3003\u3008-\u3011\u3014-\u301F\u3030\u303D\u30A0\u30FB\uA4FE\uA4FF\uA60D-\uA60F\uA673\uA67E\uA6F2-\uA6F7\uA874-\uA877\uA8CE\uA8CF\uA8F8-\uA8FA\uA8FC\uA92E\uA92F\uA95F\uA9C1-\uA9CD\uA9DE\uA9DF\uAA5C-\uAA5F\uAADE\uAADF\uAAF0\uAAF1\uABEB\uFD3E\uFD3F\uFE10-\uFE19\uFE30-\uFE52\uFE54-\uFE61\uFE63\uFE68\uFE6A\uFE6B\uFF01-\uFF03\uFF05-\uFF0A\uFF0C-\uFF0F\uFF1A\uFF1B\uFF1F\uFF20\uFF3B-\uFF3D\uFF3F\uFF5B\uFF5D\uFF5F-\uFF65]|\uD800[\uDD00-\uDD02\uDF9F\uDFD0]|\uD801\uDD6F|\uD802[\uDC57\uDD1F\uDD3F\uDE50-\uDE58\uDE7F\uDEF0-\uDEF6\uDF39-\uDF3F\uDF99-\uDF9C]|\uD803[\uDD6E\uDEAD\uDED0\uDF55-\uDF59\uDF86-\uDF89]|\uD804[\uDC47-\uDC4D\uDCBB\uDCBC\uDCBE-\uDCC1\uDD40-\uDD43\uDD74\uDD75\uDDC5-\uDDC8\uDDCD\uDDDB\uDDDD-\uDDDF\uDE38-\uDE3D\uDEA9\uDFD4\uDFD5\uDFD7\uDFD8]|\uD805[\uDC4B-\uDC4F\uDC5A\uDC5B\uDC5D\uDCC6\uDDC1-\uDDD7\uDE41-\uDE43\uDE60-\uDE6C\uDEB9\uDF3C-\uDF3E]|\uD806[\uDC3B\uDD44-\uDD46\uDDE2\uDE3F-\uDE46\uDE9A-\uDE9C\uDE9E-\uDEA2\uDF00-\uDF09\uDFE1]|\uD807[\uDC41-\uDC45\uDC70\uDC71\uDEF7\uDEF8\uDF43-\uDF4F\uDFFF]|\uD809[\uDC70-\uDC74]|\uD80B[\uDFF1\uDFF2]|\uD81A[\uDE6E\uDE6F\uDEF5\uDF37-\uDF3B\uDF44]|\uD81B[\uDD6D-\uDD6F\uDE97-\uDE9A\uDFE2]|\uD82F\uDC9F|\uD836[\uDE87-\uDE8B]|\uD839\uDDFF|\uD83A[\uDD5E\uDD5F]/,vi=/[\$\+<->\^`\|~\xA2-\xA6\xA8\xA9\xAC\xAE-\xB1\xB4\xB8\xD7\xF7\u02C2-\u02C5\u02D2-\u02DF\u02E5-\u02EB\u02ED\u02EF-\u02FF\u0375\u0384\u0385\u03F6\u0482\u058D-\u058F\u0606-\u0608\u060B\u060E\u060F\u06DE\u06E9\u06FD\u06FE\u07F6\u07FE\u07FF\u0888\u09F2\u09F3\u09FA\u09FB\u0AF1\u0B70\u0BF3-\u0BFA\u0C7F\u0D4F\u0D79\u0E3F\u0F01-\u0F03\u0F13\u0F15-\u0F17\u0F1A-\u0F1F\u0F34\u0F36\u0F38\u0FBE-\u0FC5\u0FC7-\u0FCC\u0FCE\u0FCF\u0FD5-\u0FD8\u109E\u109F\u1390-\u1399\u166D\u17DB\u1940\u19DE-\u19FF\u1B61-\u1B6A\u1B74-\u1B7C\u1FBD\u1FBF-\u1FC1\u1FCD-\u1FCF\u1FDD-\u1FDF\u1FED-\u1FEF\u1FFD\u1FFE\u2044\u2052\u207A-\u207C\u208A-\u208C\u20A0-\u20C1\u2100\u2101\u2103-\u2106\u2108\u2109\u2114\u2116-\u2118\u211E-\u2123\u2125\u2127\u2129\u212E\u213A\u213B\u2140-\u2144\u214A-\u214D\u214F\u218A\u218B\u2190-\u2307\u230C-\u2328\u232B-\u2429\u2440-\u244A\u249C-\u24E9\u2500-\u2767\u2794-\u27C4\u27C7-\u27E5\u27F0-\u2982\u2999-\u29D7\u29DC-\u29FB\u29FE-\u2B73\u2B76-\u2BFF\u2CE5-\u2CEA\u2E50\u2E51\u2E80-\u2E99\u2E9B-\u2EF3\u2F00-\u2FD5\u2FF0-\u2FFF\u3004\u3012\u3013\u3020\u3036\u3037\u303E\u303F\u309B\u309C\u3190\u3191\u3196-\u319F\u31C0-\u31E5\u31EF\u3200-\u321E\u322A-\u3247\u3250\u3260-\u327F\u328A-\u32B0\u32C0-\u33FF\u4DC0-\u4DFF\uA490-\uA4C6\uA700-\uA716\uA720\uA721\uA789\uA78A\uA828-\uA82B\uA836-\uA839\uAA77-\uAA79\uAB5B\uAB6A\uAB6B\uFB29\uFBB2-\uFBD2\uFD40-\uFD4F\uFD90\uFD91\uFDC8-\uFDCF\uFDFC-\uFDFF\uFE62\uFE64-\uFE66\uFE69\uFF04\uFF0B\uFF1C-\uFF1E\uFF3E\uFF40\uFF5C\uFF5E\uFFE0-\uFFE6\uFFE8-\uFFEE\uFFFC\uFFFD]|\uD800[\uDD37-\uDD3F\uDD79-\uDD89\uDD8C-\uDD8E\uDD90-\uDD9C\uDDA0\uDDD0-\uDDFC]|\uD802[\uDC77\uDC78\uDEC8]|\uD803[\uDD8E\uDD8F\uDED1-\uDED8]|\uD805\uDF3F|\uD807[\uDFD5-\uDFF1]|\uD81A[\uDF3C-\uDF3F\uDF45]|\uD82F\uDC9C|\uD833[\uDC00-\uDCEF\uDCFA-\uDCFC\uDD00-\uDEB3\uDEBA-\uDED0\uDEE0-\uDEF0\uDF50-\uDFC3]|\uD834[\uDC00-\uDCF5\uDD00-\uDD26\uDD29-\uDD64\uDD6A-\uDD6C\uDD83\uDD84\uDD8C-\uDDA9\uDDAE-\uDDEA\uDE00-\uDE41\uDE45\uDF00-\uDF56]|\uD835[\uDEC1\uDEDB\uDEFB\uDF15\uDF35\uDF4F\uDF6F\uDF89\uDFA9\uDFC3]|\uD836[\uDC00-\uDDFF\uDE37-\uDE3A\uDE6D-\uDE74\uDE76-\uDE83\uDE85\uDE86]|\uD838[\uDD4F\uDEFF]|\uD83B[\uDCAC\uDCB0\uDD2E\uDEF0\uDEF1]|\uD83C[\uDC00-\uDC2B\uDC30-\uDC93\uDCA0-\uDCAE\uDCB1-\uDCBF\uDCC1-\uDCCF\uDCD1-\uDCF5\uDD0D-\uDDAD\uDDE6-\uDE02\uDE10-\uDE3B\uDE40-\uDE48\uDE50\uDE51\uDE60-\uDE65\uDF00-\uDFFF]|\uD83D[\uDC00-\uDED8\uDEDC-\uDEEC\uDEF0-\uDEFC\uDF00-\uDFD9\uDFE0-\uDFEB\uDFF0]|\uD83E[\uDC00-\uDC0B\uDC10-\uDC47\uDC50-\uDC59\uDC60-\uDC87\uDC90-\uDCAD\uDCB0-\uDCBB\uDCC0\uDCC1\uDCD0-\uDCD8\uDD00-\uDE57\uDE60-\uDE6D\uDE70-\uDE7C\uDE80-\uDE8A\uDE8E-\uDEC6\uDEC8\uDECD-\uDEDC\uDEDF-\uDEEA\uDEEF-\uDEF8\uDF00-\uDF92\uDF94-\uDFEF\uDFFA]/,mi=/[ \xA0\u1680\u2000-\u200A\u2028\u2029\u202F\u205F\u3000]/;var qp=[8364,0,8218,402,8222,8230,8224,8225,710,8240,352,8249,338,0,381,0,0,8216,8217,8220,8221,8226,8211,8212,732,8482,353,8250,339,0,382,376];function Xp(e){return e===0||e>=55296&&e<=57343||e>1114111}function fu(e){return Xp(e)?65533:e>=128&&e<=159&&qp[e-128]||e}function gu(e){return e-1>>>0<127||e-160>>>0<55136?String.fromCharCode(e):String.fromCodePoint(fu(e))}var Ie=(()=>{let e=new Uint8Array(127),t=0;for(let n=33;n<=126;n++)n!==34&&n!==36&&n!==92&&(e[n]=t++);return e})();function pu(e,t,n,r,a,i){let s=e.length,l=i*90,u=0,c=()=>{let w=Ie[e.charCodeAt(u++)];return w<i?w:w*91-l+Ie[e.charCodeAt(u++)]},d=n-r,f=n+a,h=new Int32Array(f);h.fill(-1,r,i),h.fill(-1,i+d,f);let g=new Int32Array(f),m=new Int32Array(f);function v(w,A){let C=0,D=A,F=A+w;for(;D<F;){let k=Ie[e.charCodeAt(u++)];if(k<89)C+=k,h[D++]=C;else if(k===89){let L=Ie[e.charCodeAt(u++)]+2;for(;L--;)h[D++]=++C}else{let L=Ie[e.charCodeAt(u++)];C+=89+(L<90?L*91+Ie[e.charCodeAt(u++)]:Ie[e.charCodeAt(u++)]*8281+Ie[e.charCodeAt(u++)]*91+Ie[e.charCodeAt(u++)]),h[D++]=C}}}v(r,0),v(d,i);let p=new Int32Array(a*2),b=0,y=0;function _(w,A){for(let C=0;C<w;C++){let D=A+C,F=c(),k=c();p[y*2]=F,p[y*2+1]=k,y+=1,g[D]=b;let L=(h[F]<0?m[F]:1)+(h[k]<0?m[k]:1);m[D]=L,b+=L}}_(a-i+r,i+d),_(i-r,r);let x=new Uint16Array(b),T=0;for(let w=0;w<y;w++)for(let A=0;A<2;A++){let C=p[w*2+A],D=h[C];if(D<0){let F=g[C],k=F+m[C];for(;F<k;)x[T++]=x[F++]}else x[T++]=D}let E=new Uint16Array(t),S=0;for(;u<s;){let w=Ie[e.charCodeAt(u++)];w>=i&&(w=w*91-l+Ie[e.charCodeAt(u++)]);let A=h[w];if(A<0){let C=g[w],D=C+m[w];for(;C<D;)E[S++]=x[C++]}else E[S++]=A}return E}var vu=pu("!}.&u%}'&}*'~!6*)%&,~!J~!J~%L~y<~!R,~~%Lu~~#GD~~#|)1#%}^%}2%+#.##%##%}&%##%'#%##&%#%#'%#&#%#&#'#%%#&#%##%#)%''%&%#%#'%#%%#%%}%%%#%#&(23#%%#&-%0%('1#(##%#'##+%'*.:1}#%#6-+(%'%%#%%%}#L'2351&('%}&/N'(0(/*-%(%%}#'+&T%7.2}#&%&#%#36/5##%&%%#&#%%#))2%%##%&&'0~!#*+&'%1~!%).'3q?&%'1~!.##%6(~!+%%%(Gw'rT~!E#<nA%#jZ~!H%(~!42##~!*31&~!G%U~#)5~#`3~!J~!Z~%]~%Y~%C~!q~!u~#kz~%#~!6'~!D~!U~!?~#T~!c%~!G#'~%7|~!G~!J~!G&~#pb~(Df}#%}*&}#%##%##%##&#-}&'#'&%#.++}%mI,#,@&(}*%}*'%&##&#%##%}&0}#.},U},%}+%}&%}#%##&}B%(}(%}+%)})%##%#&}&%##%&}<%}>%#%&}*%}(%}9%}/%})%}*%}*%}?&}&%}3%}&*#%})%#%#)}#&#-#+*%E%%'%'#%}#*V##&##I}#&&##%&%#&&Qf%%))w/0+&%#(#.%-''''++++7}>%4'',##1,#%#&%##&#'##&#*#9)%&%}#*}%,#+P(%A&%#'&##wSD',9E00#y#@}(+}&%&>~!#~!X}#*}(&&}(&}(,%}%&#+&}#&}I%#%}%)#(},'%#*}4%%#%}(''}#/##(##),%-##%%)#&}(.}&%#&}%%}*&#%},&&}&%}#%*'#%})%}D&}&%}-&}6&#&}-,%}#%})-(~+`~,=?~I9'9%~!,#%})%})%}@%}?%}(~!?~#<~#pP~#BG~#=1#%K+~#?#~%;)~#A~#mF1~#A'~'X%'~#lR~#N~'N~#r~#m#-~#i'?%#'%~#B%##%,%#~#_%#0%~#]732~,w~2+#:&#%&'0%&>%}#>##F+)#%&&#(+_}4&}-%}(&}@&}O7Fdf0@+/v4}&WU##&/0#&'('B#%}.%}'+#%}#%%&#&%#%##+#&#)#6#'#.},%}c%},%#%##%&#&%#&~#>'*-.%##%##%}#%%}%'~#)D1}#%*&~#_%%'(~#S2%'.}#~#=##*'*-%}&'%'##&&~'E%.#&~#M4}%%##&'%#~#O1##%&#'+~#<B%##%%'%+~#;#@%}#&%#&&%#(~#H1}'%'##&&~#?A}&'~#D#%32}'&&&&~#[}'(#%}'~#;C})&}%%#%~#=&%,3}%'(#%%~#^'#&&)#%'~#Y%-~#d-%'~#^%%&#&&&}#~#b~2t*&'~&(~&@~0%~e~3}%*''0})&}+~!9##-}#%-hD*)1fC#%/&/fB#40~!+#)*4~!+~!K'&:~!/*7~!.#~!H~!L':~%x&~!H#~!*~%1~!I#~!+A~#p'~!F~~#-#~,,(~.Z~!V~%;'B'mq-W~!N~%I%#&&#&}#%},%%}'%}+X#%}#&}(%}'%}<%}#%}%%'}'%}:~![)9@~%>~#UA%-%##&~!C%~!-.9:~!1~!-^2/:a~!y,D*J#-5)/4~%23,~#G~!L1~!0X3`~!2+~!!0-~&E~!W~!o,>Y&]~%cZx_&~#O*9#A#'#+I'%#)~!0B*-5A+-((F&*M#)(-7-5+'-3a5Vi~!Y~!?+[)%3),ERHm~!+:D,VG.+)?fB%%*(%)'(#&80%1'8`K8?`+'Z#&O&'H5#*9)A%%5&3))0%39+.*7#()&&*=4@**L)<'_&*+..;(#*+)./&0#3)%')-8(4ixD(&.}%,('aI:,)%,k2231T)I'#/-W7,/'Q#.'Y24+h')37</31&83##&0#),H(?'&?/1##%#&&#%''-%&&&#(&''&#.-'%#%%(,')*'&#&#'##%(%(#%('#&##%%%%('%#%#%%#%#&%##h>w+v<ayvyvcg.uuhKr}g/v|g>u9i[~>g5uI~=RvdwEg;v/g;uk!!TTSx]@RT!U!#!@VBRUU!'UTe-d0c`e&gSdicedFcrdTaqb.kYcAohdYd@a3e+d}dMdtd.aJ#bqcK`dle/e.e'dwdPdodddjbEb}ogd^ofdpduc6j?l%d{drdqc)d7bacOdQ%T#Y)X.sR[yH>6Vyv3[xwLu>vo'!*.[yBacahoj>6Rew3[xqdZa#!a&#^(X-[yG>6Vyu3[xvg3sEr|g.u/Ri9db0T#^(Xa)!-[y;>6Vylg4wKs{JwNZt3@3r=c4Z([xlg;wKt!cpq's@v7A'*a(a+!-a#[y<3Dt?3Dt'>6Vym3[xmg9rxsNJwLZt4~?r?db1T#`-!(Xa,!0[yS>6Vz%NuQs.g4wKtnJwNZtS@3r>c4Z([y%g;wKtrdga8!a(!#&T*Y-Xa#!a0<or[yc3Dtq>6Vz43[y3JwNZtf@3s!Ju}!%Dti:pm3c_%X#tjB5pkd6q!r]u?voC'*-a.a2!0a&a+[yI3DtI3Ds~3DtH>6Vyw3[xx;:s#~<5pKJwNZtE@3r~d`a)!a2T#a.(!+U.X1[yT3Dt`3Dtv>6Vz&3[y&g9rxwzcxstPu.<rAJwLZtT~?r@dZa%!a.&^*Za(/Reu[ya>6Vz23[y1g3sEr}wkg{NuQRg{ci(U#5@b`~,cg#U(2WnH5wugcRh7dX#T(Y,a'Ta!!a,[yZ<]mj>6Vz,3[y+Pv#5ReZKu+=,%!H}7ABwkaS?Rh:BcW(X#<]mrj:ubv/ARekdg%!(!a.*Ta(Y.X1!#sP>Rl*Dt6[y>>6Vyo3Wf*jOvuumvuRgRJuq*!:9<B@bX~3jVv&v@s@5Re[d/rQt{uAvo&a&a*)a2!,0Wf!3Dt0=Bs'>6Re}3[xy~<5s%JwJZt1~Gs)c;&!#2sJkNuXvzq7rxu,Re8dka4!a8(aEZ+a@Y.X1Xa)[yd=Bs(3DtP>6Vz53[y4cX#X&Re:avRe9~<5s&JwJZtQ~Gs*i^rzvdRg+Jv{%!2sbB@bX}kdga,!Za?&^*T1/!a'Dt+[y6>6Vyf3Wf%g/u;s4hGu6?Rh-JvZ,!c%#&RoX54Rivj7uyvf8RgTKvZB%*!2sGh<vu5Rgq<=C::9bb~#dZ#T&Ta6Y.X*Dt>[y93Wf)coZ(T,6VyifluvRgC@95@B@bX~/hFu34cC#T,k/unq8w8Q5RkUklwQuzunq8w8Q5Rk8d/rJu?v8w9)-&!a0a;a&aIWejg3sEr/h1s<DtDJvyZqY5aws3Jvy!&Wei~Hr1:au5@Bag>23E~5c:Z&bX};kKv?w&unuVu5Rjc;>bs)#~@:Rh.=ay<a]C;b`}Vd6s/t{uAvoaxa()!a,a7%-a#a2Dt,[yF2Wo[>6Vyt3[xuNuPRi&NuPwpi#RoWh?vf8Ri%Jv]!%Ri:KvxD!.'2WeAjZu`q9rxu,Re7woeAg-unLq(qA_/*2Wg_g3u5q^9:4E}/jTrxrzv=Wkkd~0UX#^^Xa-a1a5T&a=U1a'*aEa]!a*aPaA-adok[y54Rn>;:p3~Dp5g9rpsFNvZqjg3uJp4~<5p0Pw;5qlJwNZt*@3p1Pw:5p/Ou!5p2JvG'!6Vye=<qnJvh_[xhg3v,Rh3kOwOw-sDuev/Re^dha[a%!%!a+#Ta7)-5TaCaO!aka!a)sf[yb2>Rl!9ARiq5E}Qg=ucRkBE|oJrJ_@Wk~@Wk{JrJ_@Wk|@WkyJrJ_@Wk}@WkzJvO_[y2g-vMRmiKuYC!)&>Ri;>Ri<@3RkNc](X#@9Rk=g5vuRmhKvDB!+'=]meg3u4Rmgd)#Y'Vz3CARmfd`a+!%T'!+#Ta1Ta6TaM-sTDt9[yA9sYd'%Y#s[[xpj:ueunaXRgEjRq,v-vuqdd2'`#6Rev<32@5>:2<E}5xIo9a*X#Y(;5RePJvD_g>vyRgNj8w)v8<wggs:RgXiZt|vjx,hSq3ah!-(~@:Ro/Ou!5RhWj^v(pyw8unRhUdx-UY#^Ua.a3a70!)%UX1TaDa)'omRiRRhE[y:3Dsz=Br,>6Vyj3[xkg6ruwjcqsrPw;5r*Ku]D'Zt-@3r(~?r.i[vwv]dU1a--U#`a4(g/vsRhPOu!5RhLj:rmu9Wo!~@:wdh@g/vsRiTjXuvvNr}:RhBj^v(pyw8unRn]dz1UYa'a+^Y(!aETZalaRY.Ta?a4[yDJw1!#qLsW>6Vyrfzq-pLflpwRe|Js>%!Dt@3Dt&Jvy_[xs~HrnjMuwpsw'RecKu+D#'!t<~Grl~?rjg5u-x,gwp{ah!-(~@:Rg~Ou!5Rh'jXuvvNr}:Rh#cW#X/c;&!#2sLi[v7u7RgpJv)(!iLrxu,Re6j7v@s@5Se[e7d`aW!Za(a`T.a#!a3!&aDa-!9)Dt_=6s+3[x~~DR|h~DS6avhGun5RkZj3w)v-]mkKunB!&*]kb97R|i<ARk<c:Z(6Vy}Juh'!wziMRoS:F|vkLuauJv5vtvQRh1d='T+Y#VyO~DR|jcF#T'7R|g97R|kJv3'!ay<Rj,Jvh&!:ReXcsa6*a+#a#_aIRf9aLRf?c,Z&Rf5Rf7c.Z&Rf;Rf>cQ#%T'p-Rf8Rf=ct#%'(*!,p,Rf4p+Rf6Rf:Rf<d~'Ua%U*^UYa(!a,-!#a4YaTalaEX0a8a<Weo3Dt/3Dsx=Br93Wen~Dr;~<5p<JwNZt2@3p=Pw:5p;Ou!5r3c7&!#:p>3Ds}KvGB)_6Vyk2sM=<r7x'eovA(!hFu1ARf}cV#X&@r5j6rvwQa^Rf3c=Za'wkghJv__g;unRggA53B9=b^}%j6uduo5Jq;!(hIv%2Re`Ou4ARe_e%a#^^^Xa&!a*a2!&a6YaP!*ad!#a:aE/5Rn?[y@>6Vyp;:pE~DrY~<5pBJwNZt8@3pCh=rt3rWPw:5pAJup_[xoNuPpF9c!#'45pD5ARn)d8#X'X*3@rU72s]h>v<<sSjJpqvewOJq/(!hNw'5ReBk0s2u3w/w'5ReE5@Jq.!a+JQ!&WeU23d(#Y&RjG5]jBk!u7w&u0udARjEe#+^^^Ub#!a2/a`Z(agT1!a-a;|@TaG!aS[yV=Re~fow'RguNuPRe?bz#'>RoUWeL>:Cbb|?JwPZtVg6ruRmzJvD'!6Vz(g/vmRh~Jvy_[y(g9voRgyx*cy(#2>Ri2B9b]~9kIw9u7rluJu3Rg]dI#a%UY'@=p%CAx.gQZ&RhwwygtRm{x5g_Z'+ABqR9Woa=Bp&dV#^*Xa'!&@o{g4v]Rk;Jv{!%Rk[wkkiA5RkiwwfUB=x,fUuqC&*!>RfTg8v0RfV~ARfSd;rJsAuAv9wR'ae+/aO!a@aza/a#[yQ@Wg!2Wemg3sEr0JvB_g>uvReWg2v+Re=KupB_+[y!2AbY~-~Hr2AJwD!(h<~El>h<~El?Kun@+_:9b`}Kg-v/Ri3g;vtwyk_9]k_d=&T#*U.6qh@Ab`|K9:H|CJv[!&3Dtex'fDwC%!Rf[9WlMd[(^X,!a%Z06Vz!@WgBg=v~Rgvg,QRe@awd,#Y+jTv|Q~EfWj]uNr|~FRfXdy#Y&^Ua%!aO.!(a)Ua;=!a@aKap!a-,a!Ta]a[rSa]p?[y82sK=Bq~;:p:~<5p8Pw:5p7d'#Y'Wf(;RnRi[u4w&RgJJvG'!6Vyh=<r#ijuuv/sIKuYD'ZtG@3p9~Gr&d2#`(g<vtRgFj`u5w&rqpxRf2CJuY!+:wfnTOu!5Rg}jNs1ucv&RfwJvA!&3@q|BDcC#T,k/unq8w8Q5RkTklwQuzunq8w8Q5Rk9dga#!a'!a=#a0!:+Tb*b@aO.a4!aba8aFJv^}?!VyR~Dr<g;u%Rn.~<5p[x'e`wNZtR@3p]Pw:5pZhNvjBp.woe_g5u-r4JwF!%DtO3:ooc7&!#:p^3DtpLuGw(!+%)Dtk6Vz#2sd=<r8d'#Y([y#<x3gJt`w@!)%}MRiowzikRij=]ilxAf3,U(#B2Rf#g0v-Rm[ck{`U#]giKv3>)!&6Ri154s,KuGB_%@r68r:dJ|t`#X(9<E|u2@H|rx3gJu?w'!+'1Nu7Reg4=H~+9<wxgY95Rm]xLggZ-`(X}U2:Ri4h<uOawRmsJv__5@bb{jbV~3dka#a'a]!,#a+U=a>b6a3b%!/aKa/)!arwve^VyJ;:pR~DpTg3uJpS~<5pOPw;5qmPw:5pNOu!5pQJvG'!6Vyx=<qoJvA!{~Jup!%@qk7Rn/KvyD!}''[xz;>wkh'?Rh,x8gyt`w5D!&),(SgyccRgztJ@3pPB5p#d'(Y#<]mmifubw&RgoJvE&!82s^JvF&!8Rf,ADb]~;x=h'rNu]vK!,%'*0RnORh)4Rh*AqQg-vaRnNg;wHwkh'ba~4cE#Ta*x3gctyw@'!+%RnFRnD<4Rn@hFvK5RnCxWg[#`&a0Ua()`1Rm75Rg[c]%X#qi8Rg^NvdRj>BwzgZauwji7Rm6A4wgg]d1#&(*,.0a#Rm;Rm<Rm=Rm>Rm?Rm@RmARmBe%#^^^Xaea?aC/b+(,!a+a#!a/!>a&Ta<aKbD!2wphBRnk[yPw}hE|.=Br-3Dtm>6Vy~g6urRf.x,hPrNav!%'RnqRo%Ro#Nu;q[Pw;5r+JwNZtM@3r)d'#Y'Weh;xChL#`&RnmRnoKu}>%(!Rne~Bs-;2wjcussJv+'!aYSO}6@B<5?ba~8LrNvj!.%*ROwungw~ng~:9;Ri^>wtnig;wHRnixDh@|(UZ.x1h@|)!#:2<H|*xHn]#-UX'3Ro)z=iT}6ARns=Bwsn_wpnaRncw]aR(#UXa&Ua*a/=]iPd'#Y&Ro'WnXf{QRm2hNvj]nZd`'T~&1`{|`#9b]{}c:'!#Wl{>@=be}]?cl{{U#:5Abb}Jds#^YaF!a*b4a#a3aPa>&Tb!bH!*a_!Eau?/a&RjY<]gj>6Vz*;:pe~DrZg,QRj1JwNZtX@wihspcJvZ&!VyX9WmOJu|!|N2WmHJvh&!]ht~Bpbcn&T(!#RmQ<s7Nu;padH#X'`+WmJ@>RmKCARhnKup=!)&Wf+:RhqNuPpf9c!#'45pd5AwghpARn(Ls@w!%,)!RmP@Wfe<E|IJva!&WmNg8vsRmLd`*.`#Y'Xa!axRn*]hrA8Rhug5s@rXg8u!RmMd8#X'X*3@rV72smdI*#UY&RmICARho~GsgxVgd)Ta'U-Y&Xa!T#RnEWnA@Wffg1uDRi0hFvK5RnBxGnG&#`%owp)@wsf+bX}Ze-*1!a*^^^Ua|!#a.aq&Ya2!a>.a6!a:aO`aJDtL[y`@Wg#>6Vz12@wzoYRoZNuPRi!NuPRhzg=ucRi,@=b`{Yg=ucRi-ACJvB!&Sh[ebSh]ebi`wUuFRm4Jw2_[y0JvB!.<Ju(!&SoG}6Shd}6<Ju(!&SoH}6She}6Kur@._g5vHRieJvx!{L2G{Kx6gd'T#?Rh82Wi5cZ#X(g1w)Rm5dW-Y(Ta#!a)!#aYa=wnfE=su2>>bU{0j9udv:<svj8uQv-7RgHdE%#^'sq9sp=>Bb_{TJv`!&g/r|snj6v(us5d,#Y(56H}[978H}]Jw5!&g1rushJvB!+j;v{u5?zDhd}6}bj;v{u5?zDhe}6}ce*#`(^^^a[aea!=!a6a*aoXb1a.!aAbL!b>,b'aL!aV@Wf|2Wlg3[y/JwNZt^@3piPw:5pgJunZou3@rsJva&!Vy_g<v~Rm#JvG'!6Vz0=<r{Ju{%!:pj@WfsiXuJu3Rm:JvZ&!WfA~Bph@c4Z&Dtwax5rubx(#:awRk1@d,#Y&RfjRfid1#,Y(@Wfp2Wlrg5s@ryKu[@!,'=]ig9wlk?Rk>g5u-rqJvy'!@9RkQcH(T#=>Ri~@<wkj(Wj(KuZB*!&<7rw@9RkRcH(T#=>Ri}@<wkj)Wj)dg(Ta2Xa9X#`-!a*CARhg@@=I}d9x;c~#X%so=<sj>2@@=aybb}XjWv0Q~EfEj3vLv;<d,#Y(56H}`978H}_dgaPaFa'a/!#a3Y0a_a;a|!1(a7-[yE3[xt;:pJNvZrrg3uJrvJwNZt=@3pIh=rt3rxPw:5pGOu!5rpJvG'!6Vys=<rz@c4Z&Dt(ax5rtJvZ!&~BpH@wsfNg-vaRlNci*U#=<wei<F}a5@Jq.!a*JQ!%@qZ23d(#Y&RjH5]jCk!u7w&u0udARjFd/prq=tyvpaEa(a:.!a1aZ(@@=I}:9wpd%=<sX55w_h}@@=I{t=ay<aU@@=I}T=ay<2@@=I})?C9:9au@9Cb]}DP~=x-fAZ(2Wl1=ay<aU@@=I}>5@d##Y+jTv|vV~EfFj]uNpn~FRfGdgaK!Z2&!a8a-Tb({E!acTbM*!a(DtY[yYd'%Y#sl[y*hHvh>Re5x2c{Z}.j4uCvcawRiMd+#X+_x&d!},<5RkX;2Hzw@x,gavfB-!{CcF&T#Roe;RodwWbBg5urRgaKvHC*_6Vz+<4opieuew&Rmq@d]&Y)X,T#X0Rh}<BqP=4qS9:ReMg/ujReNJw0!/<Jui%!bd{kawwnemRelAxUa?a3#*.&UX(Ya+a/RhvRnQ<o}9Wmtd-#Y&RgSRmw9;Rmxay=Rmyg-vaRmuxEhSrNu,v-voC!%(aR.a(a7+1Ro1>Ro5CE{A9b]{@;5x#eO{:g;urRi+KrNA!%(Ro3>Ro79;Ri_Ku@>{;&!x%gX|{KunA_+g5QRj/g3u5Rj#g>uERj%wio/xRhS&!,!#^1U}wba{8>>@=be}qC@:D5ba{7Ku+A&!}x?ba}t>>@=be}se(aA^^^Uat!b0#{pa+awUazbGa#aLb9bgaWac'a5TbS=Br!d1#`%scp_Jvl!#rT>Re0JvX&!VyN=H{Fcm#U&:pY=ReaJv2&!]h0=]nUJvG'!6Vy|=<r%JrM_=]h2@Wlud'#)U'Wf'b]{i=]h/Jvh!&~BpWg=v]RnMx+ny#'Nu;pVwjnu=]nwxJnx,T#`&Reqwjnt=]nvieu9vrRjLLuYwP(#+!th@wih5pX~Gr'g5v/Rh4KunA'!-CARnP@wwiN:Rm_9x'cvw>!|l=<saKvAA!0&3@q}>w^e1bp#&Re2Re3BDx7gH#T|f5H|eKuZ>!%(:qNAH{]Jv6!+3B2B9=b^{X<5<B92:E{ZLvhwA(a;a%!igQuyRmad+#Y}m@3Rh5d8#X'X*:AqUAHzmaxwbh<aXRnVcF}RT#Nw&cj#U(BWnug/vsRntdka)(a3+.Zb7aYYan1!bVa@Xa}[y^@b[{G=H{+hFu73Rj&Pv#5ReQcK%T#sig1v{Rj'Ku+D#'!t]~Grm~?rkKuMB!01d5#`'Vy.ta3Dtu~Hroc8#'{^45s85AwZbP&!#Rn!wghxWn#KvEA!)&2RlA2RlBx:h|#(T,=]j09Wobz>x]z/@awRoTd+#Y(az]hFhCrm4d,#Y+jTv|Q~EfMj]uNr|~FRfOdCa!Xa9_X#@<plJvf!%b`{(9;Rgwc;.!#2x7cw#T|UDb]|T5Ju={(!=@E{&Jv)&!Ab`{'awJvf!~*>>@=be{#KuY>!+&4Ezyi[ugv&RjIdea+T)#UXa&T-T&a!Rh9auRmW=]kLg5vuRn+g3u4Rn-Ow6ARn,hHus5xNk?#UX(U~)/g8v0RkD~AwkkF?Ri.OuNBwkkA?Ri/d|a2`a*^UYa.!aBTZaTa'Xa;!(!2!-a#b2[yC>6Vyq3[xr2Wi?g1rusVh%s?DtF~<5rbJs;%!DtBfswKtCj[uvuSsEu3RgVx3o:u+wN'*Zt;@3rd~Grh~?rfg8w)Lq)qE&-a%!>bI|`jWv0vV~EfCjTv|vV~Ef@j]uNpn~FRfBcK#T']gWNu7x,k7q4ai(0!hHv8<RhmkMu9vrsBuev/RhlCJvB!,g<v{wchh~@:Rhji[vrv{wchi~@:RhkdS&a5UY#Ta!RgPwwiI5BwciI~@:Rh`x'iJvj'!5]iJPu8Bwch]~@:Rhach)U#h3rp]gLh@t|Ax,hTq3ah!-(~@:Ro0Ou!5RhXj^v(pyw8unRhVd|)`,^UYas!a?/a2Z'a^Ta{Tb7Ta(a#!a,Wf&9sZ3DtAadamov=Bqt3[xig8vsRm~>waiL2b`{QJv*_Ouv2qgj<v]v2BqfdR'X*X#Y-@3qr~Gqv~?p6hHv-]glPup5Lq+q?_%*b_{qF{n9b^{rOu4ARhpKvCD!+&~Bqp:5Dbb}nwoiKl&unuTuBv]v+ueunaXRf0=Jvh!0nKufu8v1w&w7q%w&uHrz:Rgnj5w,uxDJq/(!hNw'5ReCk0s2u3w/w'5ReFd>Za&!*UaA=<wkgsRnSJv^!%Refifw3vyRgOKu_B'!,<]gkiiu:w&Rh<=C@a^<B57@2F{[<B5@aW:=3away9A5aW=<B=C@a^<B57@2F{Ie-#`(^^^bCara.b8aza6!/bZ,!adTbnTbOb+aFaS!aAT9@Wf~2Wli3Dtl2@d,#Y&RfnRfmJwJZtN~GqyJva&!VyMg<v~Rm%iXuJu3Rm9Jv[_=]ih9wlkDRkCd1#`(@Wg>2Wls3cH#T(@<Rj*=>Ri|b~'#23s9h<~El.d'#Y&Dtxi^rzvdRl#d*#U%(o|B2s`hJwSaxRmDKv4B&!1:Rmdd5#`'Vx}to~Hq{x'f1v3(!BA5ba|bJv_&!Wfug1v]ReIdO+U/Y#&G}-8wze=Rh{g1v]ReHg/uQRf/by#)ibQwERl/cH#T(@<Rj+=>Ri{cNu+vlax-!(#a0qa9<Rii2;;bU{H;x<i=&X#Rk`<4wwi=C9H~8xAI(Y#<azRi@45wXI<B9;5bb~7dL(X#Xa(+!aL6Vy{g5QqOau:5au2@ay547EzbxOcU(UX-T#Ta#:Cbb|A?wjh/b_|SOw6ARgtihr}u7Rhy<d1#T)X1@@=I|~=ay<2@@=aybb}Sj3vLv;<d,#Y(56H}A978H}@dGpvs@uAu`vcw9*!aFa+ai%(b!aXa8.a?a[ozWey=sU2@G}Nch&U#Rf_WexKu+D#'!t:~Gr`~?r^j]uNr|~FRg*j^psurwJt|RmcKv)@&!)7Rkv~Br[@wxfO:Rl3co#U'6Rezj_q#vIuavjRltwzeyh@vr5JqD0!>aY?C9:9au@9Cb]}9cl#U*5;5<H||jbuus1ucv&Rfvg1v~d/pppzqFr^a--a~!aMat1(hFv;Wiz@@=Izoj5uuv-7Rix~Cw`fk2WlVcZ#X,k)u3vWs@u2]ktg;wEx'fBq(_2Wg/jTv|vV~EfoJv]!15x'hzqG!(P~EfU~CRl_j6v(us5x4i-#T(2WmZ?C2F|d>Kq<aj1!*jTqIsBv=Wl`~Cw`fi2WlWj`v0u*~>RlR=c>Z,k#u3vWs@u2]kr<c1Z+jTqIsBv=Wla~Cw`fm2WlXdmb3!a{(arZa`bkTa%TbQTa-a9+c'!aM!/[yL=Bqug.w'RifhFvyDRj.g>vgwyk^9]k^Jv3_@WfbAARkhJw2_[x|JvB_wkoIRoKwkoJRoLd'(Y#<]gm=<9<H|yd'%_X#skDtb3awwqkgNulRkgdB#^',9:p'hJwSaxRmEBwVb8@4=H|qLu+w50&!)@3qs~?pU>Awwn;;Rn=c:Z'ARn<=<qwKvC@!/&~BqqJv6!&]eVb^z^xRge'/a%+^`#Sge}6<4Rn3=]n0Pw2>Rn8Jw0!&>Rn:>Rn6cY#a7+!a&=<wkaNw~h3z_c5Z{=wjh#=]nLKv^D!&)Vyz=bW|swYb<WetcG#T(2wxa@qVx@gD#Y&b^|V5JwG&!5bb|pg/w&RgD@x=kHs=uAvn!a%%/'+RmSRh694Ro`g-vaRmRhHv-]mlxCcS#`&ba~.5cD#Ta)P~=d,#Y(56H{>978H{Dd_#{2^Y%_+qbbb{6g3sERhsbU{?dfa.,`a(Xa<!aiX#(55RiG54RiHcI#T'WiU3RiVNvdwtfcRlKNvdd,#Y&RlHRlExQgf.1*^T'X#Sgf}6Wn4=]hfPrk>Rn7Jw0!&>Rn5>Rn9Lunw?&a2!,5<oq@@wqfdRlJj5Q~=d,#Y(~ARfcOuN]fdDKw;ay(}i!547E}j?cI#T(@5bV}iCbV}hdv(^^Tb?a40,b##Tbo!a*bR!a<b|a/!aKai!aU[yK=]o^g:v>ReGJwPZtK<7Rh+h<~El,Pv#5ReR@awwxjCg,ulRjDJv6&!]j!z?aQeeg>w=Sh<eeJw;!&axEzOg,Qosc!#*:wkeJ]eJ>x'h-u(!%Ro.w~h.zPdNZ(X,Ya![x{;9ReY;wkgxRiF:x?ap#Y&RmUg<s2Rkod]+UY0TZ'!a&A9sw<=bczLNvuw{gqzNhJwSaxRmCKuLay!#&s_Rf-55b^{uJvZa!!c%#(55Ri654wmiu5RiuawLu,vp!+}^%b_}Y9;wkgxba}o>A9:=b^}zKuh=a''!3awRk3c*'!#aHRk6c+Z&Rk5Rk4Jv)&!awRjSawd9*`#0?C2@EzMj8u<uJ5RmbjQrquJu3x,k>uq@_+=ayb^|W~ARkEOuN]k@7dhzV^X/X&a-#zRzSb`zXcJzTT#2WkVKvDBzW!%FzY9;5bbzWjQrquJu3Jw3%!b`zU=ayb^zQd:#X(T-a!6Vyywxh}=b]{Jg=u1RiAdGp~qHtzv!w(wA+a+a;<!aJaYai'anasb(=azRmV:Cbb{MLq2vb!%')RjuRjrRjtRjqx3jnqCw3!%')Rk(Rk+Rk&Rk)Lq2vb!%')Rj{RjxRjzRjwLq2vb!%')RjsRjpRjfRjex3jcqCw3!%')Rk'Rk*RjkRjl9<CbbzfOu4ARhxLq2vb!%')RjyRjvRjhRjgx=joq*uKvb!%')+-Rk.Rk%Rj~Rk-Rk#Rj}x=jdq*uKvb!%')+-Rk,Rk!Rj|RjmRjjRjidAq&qKs@uAv8Aa.'*-a@a&0!aM@a5[y73Dsy3Ds|3Dt):wxgI2sHJwJZt.~Gqxwsf0ikrzt}Rl0Jvy_[xj~HqzKv_A|D!&WfP8axRoVcf,U#k(v]v+ueunaXRf1Ju}'!g8u#Ri=jQw!sCunLprq>!,')~<5qeGzq9F{W=c##%s5au:5aU3CBE|;d4#X(D!a&6Vygx(b;#(=]ed?C2F{N<capoq2r[a&!aPa9,'Pw;5s:@@=I|,55w_h|@@=IzcP~=x'fCqB_2Wl2>aU@@=I|1OuNBc1Z+jTqIsBv=Wlc~Cw`fl2WlZ~AcTa%!Z+jTqIsBv=Wlb~Cw`fh2WlYk+uNqJsBv=WlSg,u3dca3#UXaMYa)TaB-=cM|7T#<bI}l5@B932:aV2G{BOuNBJq:|M!5Ezt=<B=C@a^<B57@2F{v>cB{/T#=ay<bI{3Jv6!a.6BKq0ah&+!5E}HP~Ef{978BaU@@=Iza<7d#.Y#978BaU@@=IzH~AJq0!(@@=IzG978BaU@@=IzFe,aU*Y&^^^bvJb,b:bFad!a,c2Ta>aL.bo6!a#CbTa'T#Re{2Wlh2@G{yg6t~Ro_NvdRfticuRQRllJv3&!x&c|zs@Jw3!%RflwpfkRlpKuL;%(!Re<@G|C2GzdhIvuBwgjAg-u0RjAKQB%!(GzZ@G|5NuuRl7d='T+Y#Vy[g<v~Rm!==G|>JvA!)@wma=]m1ifuaw&RmnLs@vT'!|/+[y,g:v>ReTJw1!#qX=x!eC{bLu+wT&)ZtZauq_~Graci&U#F|89:r_Lupvq!.)&2RlG8RfaC=x!eF{_h?rpWlmd&'!#X|&]k::xJey#`'T|+<E|&2@H|%dE#(^,g;u.RiEg6vjRiC9xCkA{O|zY#g=ucRmXKs0@!&*@G|m@awRknJuh!,3d(}gY}eJvj!%Rm):Jw3!%Rm+Rm-Ls0w(&!a(a#@b[|6cZ#X'7RkxWgAOu4ARn'dH'U#Y*Vz-Wm'CARm}d]*#a%^a*T'aK!a<9bV{PC=p*Jw4!&SgxcbB5r]idw(wBRmF7xFkt#&`(Rm/Rm8E|!JuY_9:Rl5=wrgr2:bbxd@xXfB(a*#T+!.X0X1Ta/a'T&RlDRfL>RlyARl9b[z[>RfZ:RlL:RfRwlg/ARl;9;RlxKv,A/!%7s69<74=BA5ba{-8Bde#`a<XaKYa1,a'P~=wxfB2bZ}}?C972@@=I}r8@55B9;5bb}G978B2@@=aybb}3j3vLv;<Jw3&!>Rfk=ayb^}4~Ad1#`*@@=aybb{w2@>==<bbz]dx+UY#^UaF!a9!bB'Ya1.!ajXa#%olRhD[y=3Dt#Ov5BrHKuMB%!(Rf^Wep~HrJwkiQjKr|~FRg)Ku+D#'!t5~GrF~?rDdV)UY,Z/_7RkuG{<~BrBg,rlsO:235B@bX}|d?a1!#`(6Vyn5@d##Y+jTv|vV~EfIj]uNpn~FRfH7Lq2vb1!a9-978BaU@@=Iz9978BbU}#~AJq0!(@@=Iz8978BaU@@=Iz7~AJQ|}!978BbU}!JvkaK!AdUa21-U#`a+(g/vsRn~Ou!5RPj:rmu9WhOjXuvvNr}:RhAj^v(pyw8unRn[kPr}p|u7vwv]RiSBd;pppzq@qHQa?(b.!a.a`@.|xa(hFv;Wiyj5uuv-7Riw~Cw`fg2WlU978BbU|wOuNBJqG!(P~EfD~CRlQcZ#X,k)u3vWs@u2]ksg;wEx'f@q1_2Wg.j]uNpn~FRfqJv]!15x'h{qG!(@@=IzK~CRl^j6v(us5x4i,#T(2WmY?C2F{1>Kq<aj1!*jTqIsBv=Wld~Cw`fj2Wl[j`v0u*~>RlT=c>Z,k#u3vWs@u2]kq<c1Z+jTqIsBv=Wle~Cw`fn2Wl]dn1#c(a(b^a2!b/bAT(bj!aDa7bu,a_a{c0!2T0g:v>ReD2@G{42@G{5~DpM~<5rc=Bx6i>{RT#RnI@zCx]y]z:2Jv[!zr5Awyk]9]k]dD(Y+X#6Vz.g=wKtgwhaCwgmTWj2Lu,w%_+/[y-B;b^xeg3u3Rj-2@bX{*KrJ<!+'@Wg(g?QRlC@Jv`!%b[zIwsfII}8JQ_@w|kW|=Jv(%!AqcOuNBJvEzh!bYzjLs@wP#(0!oy@>RkdJwMZtc3Dtd@BcG#T'9bWxg2@2Fznd*#Y+;2x'c}w<zizixNgwa#Z'U+!/!a'!a+w~g~z6wcn{Rn}wcnzRn|5Rh%=]nJg5vuRmvNvdRlvcprJu}w*az*a#!%.a.'Bot9qT]kj@Wg'ay2Gzv@Jv`!%b[zEwsfHI}1;ck#Ux`<Cbbx_Lu+w!a&0*!wko*wwo,So,}6Juqxf!E}PigQuyRm`d3(`#8>Rn%:A5B;bZ~%KvhCa!a2!x>k7#Uxb@b{#xaRk7Jw0!)>wwhlShl}6>wwhmShm}6CJvB!.x'hhvj{!!5Bwkhhbaz}x'hivjz~!5Bwkhibaz|xEhTrNu,v-vpD!a%&/)a3a.,%Ro2t[CE{)@3re9b]{%wjo09:rgc:Z&Ro6=<riifuaw&RmoKrNA!%(Ro4>Ro89;Ri`dSaL'UYzxZb)7Rka3xRhT&!,!#^1U}vbaz{>>@=be}yC@:D5bazzKu+A&!}{?ba}y>>@=be}wxBh[t`u~vJvr!%a!a()a,a0a4RoC=]o;Ju(!%RoGRhdwjh`=]oAg>w#Ro?g5vuRo=NvdRl|Ku]C.!&;RoEJvB!%RoORoMBx'h[v+_?w~h`}~5?w~hd~!xKh]oiptu-utv.vp!#%&a30a@a'a+(a/aOp(o~p!RoDJu(!%RoHRhewjha=]oBNvdRl}g>w#Ro@g5vuRo>c[#X']o<CauRoRAd-#Y':RkpauRoQKu]C.!&;RoFJvB!%RoNRoPBx'h]v+_?w~ha}t5?w~he}ue!/UbhYacXaW^Tc&a;b:a-c/#b&aja1(!cL+!bKbt!bmcRc9aIc?8[yW3Dtt94Rg`Jv}!&SiRMzBhEebShEMNuPRe>x7gL#TzuwjirRipc<Z&>on;>z=h-MSh.Mwqczx'a7vj&!>Re4@=ResJt__NuPRi*NuPRi)j]uNr|~FRfzKrJ>_+@Wfy@Wf]2WocKrJ<!+'@Wg%g/QRl@@Jv`!&awRl<wsfFIzgLu(w*!.*&ShBMwvhIRhI9;RhNx1hK'!#Sn]Mx1hK~0!#:2<H~7cNu+w7D*'1ZtW>Rn1~?rOc:Z&Rn2=<rQ<7wjh&=BSnLMc]#X(6Vz)w[b=a!U#9wzgMc3#&(RgMRitRis<x,gKt`ax!&+SioM=BSilMc3#&(RgKRinRimKurB,!&SiQMzBhDebShDM6BJQ!(P~Efx978B2@@=I}WLrJw!!,a*&@G}O@9wkibRid@@x'fKwC!&SlDMSfLMjUv~Q~EfKKv3@a+!(hFv-]mpx/hYZ(C5RiWz<o/MwkhY?So/M@x,gbvfB*&!SgEM:SoeeehFu3:Rgbda(,^TZa)X/7Sg[eb:2RgI~BrMC@wgkc:wwkcRerx3h(uUvK!&*,SnOM4Sh*MArRg;wHRh(x=h;rJvPwI!a4',a'0@Wg&=BSh/Mg>w=Rh=g3w*wwgGRgGcW(X#;Sg}M2Gzk@Jv`!&awRl=wsfGIz`dKZ*T'Y-:RhR7RhQg5u-p`j6v(us5d,#Y+~Awkia?RicOuNBwkibba}Ld6p~tyu_vbAa'a+!a/'a3aEa8a!>Sh,ebJv{!&Sh@ebSaReb9;SgwebNuPRi(NvdRl)NuPRi'hHu^<Rm^Jvv_@Wl(g;u1Si/ebKu'B&!*Sh?eb@Wl'z@aPeb95Si.ebcpputyvjB)!,&a+0a%ShAMWeK@G}C@WfJ9;RhMwvhH9w{ia}ix,hJvRA1(!zAn[MRhHx1hJ~*!#hFv(BSn[MBJQ!(@@=I~'978B2@@=I}2db.Ua<'X}+T#a0XaG2G}E;wkg|wuh!Rh!x,hZu,@)!&So0MVy)C5RiXACJvB!&5RiY5RiZg8w)cG}*T#2@bU}=KsA>(!a.3wkhZba~(x,h^u(A!&(SoCMRhb5Bz=h[eb?w~hb~6x,h_u(A!&(SoDMRhc5Bz=h]eb?w~hc~6e)aA1T#T,^^^c-bMb&blcPaP(a/!0!bA=b5c@a(!bfbrc#2afwmhARnjwchORnp2Wlf3DtsNvdRl-2@wpa<]m0bx(#:awRk2@Jw3!%RfhwpfgRlnKQB%!(G{V@G|'NuuRl6d='T+Y#VyUg<v~Rl~==G|<Jv+'!aYShC}6@B<5?ba~8@Jw3'!g2QRljhLrpWlOd+#Y'g.w'rIg>w*wgj@g-u0Rj@Lu+wT&)ZtUauq]~GrGci&U#F|39:rELrNvj!.%*RhCwunfw~nf~:9;Ri]>wtnhg;wHRnhx3hDs@v~!/+'@Wfr@9RkSNu&Rlo=@<5GzoKs0@_+@Wl+@awRkmJuh!-3d(}pY#qWJvj!%Rm(:Jw3!%Rm,Rm*de&!1U-U#`)Re;@G|.@9Ri82@wjfvRlq=@<5GzpLvOvr!).&2RlF8Rf`C=x!eE{.Jw3_g2QRlkhLrpWlPde(!#U{s,UXa*Ta'[y'g:v>ReS;x0PZ&RnlRnn~HrKJw1}f!=x!eB|2w]aP(#Xa&a*Ta.Ua2a7=]iOd'#Y&Ro&WnWg;u.RiDg6vjRiBNvdRlzhNvj]nYJuW_2Wm3x)kFze{9d])!a.!,Y01!#&aC!a3RndC=ox~BrC@2b^{pg,rlse7x'ksuq!%Rm.E{xidw(wBRmGx9o+)X#wwo-So-}69:Rl4@xSf@a#XZ'X)X,Ta(/ARl8b[xc>RfY:RlI:RfQwlg.ARl:9;Rlwdn'#^XafaQa1X1TaHTa)@b[{zcZ#X'7RkwWg@Ou4ARn&x)kG#{,g7u/RkGdH'U#Y*Vz'Wm&CARm|bx#(A]gUbUzJj9Q~=d,#Y(56H}l978H{U7d,0#U*2>ABb_xZ978BbU{e~AJQ{g!978BbU{hxMh?ad{oUYZ.x1h?{l!#:2<H{mx3n[t{vl!,&a%3Ro(z=iS}6ARnr=Bwsn^wvn`Rnbd`*T}B0!#^X'BG{c9b]{a>>@=be}F?JvS!&BG{d7BG}(Bde#`a1X,Ya@!a'P~=wxf@2bZ}I56B2@@=aybb}08@55B9;5bb}<j3vLv;<Jw3&!>Rfg=ayb^}&OuNBKuLA!)a!P~=x#fD{f2@>==<bbzl?C972@@=Ix^d6rSu,v7w*C(0a)a6#B+a%!sQ[y?3Dt%3[xn~<5rLOu!5p@Ku+D#'!t7~GrP~?rNKvlaya7'!h+v-5qMg=t|cd,U#5AAaa5Abb{S@52B5@a[@52B5Gx[iXueu;d<#`a(!/549C;ag>23ExY5@Dah89b^~689Jv)!~2b[~1Lv'w(%*!a#bX|aPrmawRe]keu7uhv-q6rxu,q`xTo]/a5aU!bNaDXbi!b-!ao!b<bwA!#5@B932:aV2G|:d-)Y#hJrL>RhG<7@C5<H|_=Cau:5aj5@B932:bJ|ng>vIbs)#?C2F|9jPv0w.vISh-MKvUaz(.!9ABbb|[5;5<H|Eg>unwfh;9:4E|YjQsBt|vjx'hYq3!(?C2F|J:2<BaY?C2F|GOu!5x,g|p{ah!-(?C2F|c9:4E|OjXuvvNr}:Rh&i[w*t|cd+U#jJvsu)vsSn~Mkfrmu9p}u7vwv]So!McW#Xa!ax5@A5aY:5;5<H|>kJv~vYrquJu3x4ib#T)2@SmZM?C2F|Bj:rmu9@xPhI(a*a#U#`a3-5Abb|L~@:RhK9:4E|0@52B5G|#C::aY?C2F|-:2<BaY?C2F|.5Jvk!a)javYrquJu3x4ia#T)2@SmYM?C2F|HAxPhH(!a#U#`a*-5Abb|4~@:RhJ9:4E|R@52B5G|F:2<BaY?C2F|Sc^#Xa2j=Qq5CJvB!-g<v{z;hhM?C2F|Zi[vrv{z;hiM?C2F|XKsA>!a)-g<v{z;h[eb?C2F|]i[vrv{z;h]eb?C2F|^iZu.vix,hZq3ah!.(?C2F|QOu!5ShXM:2<BaY?C2F|P",13494,2713,49,25,61);var se;(function(e){e[e.VALUE_LENGTH=49152]="VALUE_LENGTH",e[e.FLAG13=8192]="FLAG13",e[e.BRANCH_LENGTH=8064]="BRANCH_LENGTH",e[e.JUMP_TABLE=127]="JUMP_TABLE",e[e.VALUE_MASK=8191]="VALUE_MASK"})(se||(se={}));var fe;(function(e){e[e.AMP=38]="AMP",e[e.NUM=35]="NUM",e[e.SEMI=59]="SEMI",e[e.EQUALS=61]="EQUALS",e[e.ZERO=48]="ZERO",e[e.NINE=57]="NINE",e[e.LOWER_A=97]="LOWER_A",e[e.LOWER_X=120]="LOWER_X"})(fe||(fe={}));var Zr=32,yu=21,Yp=2097151,yi=2047,_u=0;function Kp(e){let t=e>>>yu;return t===yi?_u:t}function xu(e){return e-fe.ZERO>>>0<=9}function Zp(e){return(e|Zr)-fe.LOWER_A>>>0<=5}function Tu(e){return(e|Zr)-fe.LOWER_A>>>0<=25}function Jp(e){return e===fe.EQUALS||Tu(e)||xu(e)}var mu;(function(e){e[e.EntityStart=0]="EntityStart",e[e.NumericStart=1]="NumericStart",e[e.NumericDecimal=2]="NumericDecimal",e[e.NumericHex=3]="NumericHex",e[e.NamedEntity=4]="NamedEntity"})(mu||(mu={}));var _i;(function(e){e[e.Legacy=0]="Legacy",e[e.Strict=1]="Strict",e[e.Attribute=2]="Attribute"})(_i||(_i={}));function Qp(e,t,n,r){let a=(t&se.BRANCH_LENGTH)>>7,i=t&se.JUMP_TABLE;if(i){if(a===0)return r===i?n:-1;let l=r-i;if(l>>>0>=a)return-1;let u=e[n+l];return u===0?-1:n+a+u-1&65535}if(a===0)return-1;let o=a+1>>1,s=n+o+a;for(let l=0;l<a;l++){let c=e[n+(l>>1)]>>((l&1)<<3)&255;if(c===r){let d=n+o+l;return s+e[d]&65535}if(c>r)return-1}return-1}function bu(e,t,n){return n===1?String.fromCharCode(e[t]&se.VALUE_MASK):n===2?String.fromCharCode(e[t+1]):String.fromCharCode(e[t+1],e[t+2])}function ev(e,t,n){let r=t+1,a=0,i=r;if(r<n&&(e.charCodeAt(r)|Zr)===fe.LOWER_X)for(r+=1,i=r;r<n;){let s=e.charCodeAt(r);if(xu(s))a=a*16+(s-fe.ZERO);else if(Zp(s))a=a*16+((s|Zr)-fe.LOWER_A+10);else break;r+=1}else for(;r<n;){let s=e.charCodeAt(r)-fe.ZERO;if(s>>>0>9)break;a=a*10+s,r+=1}if(r===i)return 0;r<n&&e.charCodeAt(r)===fe.SEMI&&(r+=1),a>1114111&&(a=1114112);let o=r-t;return o>=yi&&(_u=o,o=yi),o<<yu|a}function tv(e,t,n){let r=vu,a=e.indexOf("&");if(a<0)return e;let i=e.length,o=0,s="",l=r[0],u=l&se.JUMP_TABLE,c=(l&se.BRANCH_LENGTH)>>7;do{let d=a+1,f=e.charCodeAt(d),h,g;if(f===fe.NUM){let m=ev(e,d,i);h=Kp(m),t&&h>0&&e.charCodeAt(d+h-1)!==fe.SEMI&&(h=0),g=h===0?"":gu(m&Yp)}else if(Tu(f)){h=0,g="";let m=f-u,v;if(m>>>0<c){let x=r[1+m];v=x===0?-1:c+x&65535}else v=-1;let p=0,b=0,y=v<0?0:r[v],_=d+1;e:for(;_<i;){for(;(y&(se.VALUE_LENGTH|se.FLAG13))===0&&(y&se.JUMP_TABLE)!==0;){let S=y&se.JUMP_TABLE,w=(y&se.BRANCH_LENGTH)>>7;if(w===0){if(e.charCodeAt(_)!==S)break e;v+=1}else{let A=e.charCodeAt(_)-S;if(A>>>0>=w)break e;let C=r[v+1+A];if(C===0)break e;v=v+w+C&65535}if(y=r[v],_+=1,_>=i)break e}if((y&(se.VALUE_LENGTH|se.FLAG13))===se.FLAG13){let S=(y&se.BRANCH_LENGTH)>>7;if(e.charCodeAt(_)!==(y&se.JUMP_TABLE))break;_+=1;let w=S-1,A=v+1,C=0;for(;C+1<w;C+=2){let D=r[A];if(e.charCodeAt(_)!==(D&255)||(_+=1,e.charCodeAt(_)!==(D>>8&255)))break e;_+=1,A+=1}if(C<w){if(e.charCodeAt(_)!==(r[A]&255))break;_+=1}v+=1+(S>>1),y=r[v];continue}let x=y>>>14,T=e.charCodeAt(_);if(x!==0){if(T===fe.SEMI){h=_-d+1,g=x===1?String.fromCharCode(y&se.VALUE_MASK):bu(r,v,x);break}if(!t&&(y&se.FLAG13)===0&&(h=_-d,p=v,b=x),x===1)break}let E=Qp(r,y,v+(x||1),T);if(E<0)break;v=E,y=r[v],_+=1}if(g===""){let x=y>>>14;x!==0&&!t&&(y&se.FLAG13)===0&&(h=_-d,p=v,b=x),h>0&&(g=bu(r,p,b))}}else h=0,g="";h===0||n&&f!==fe.NUM&&e.charCodeAt(d+h-1)!==fe.SEMI&&d+h<i&&Jp(e.charCodeAt(d+h))?a=d:(o<a&&(s+=e.slice(o,a)),s+=g,a=o=d+h),e.charCodeAt(a)!==fe.AMP&&(a=e.indexOf("&",a))}while(a>=0);return s+e.slice(o)}function Jr(e){return tv(e,!0,!1)}var Eu;(function(e){e[e.XML=0]="XML",e[e.HTML=1]="HTML"})(Eu||(Eu={}));var Su;(function(e){e[e.UTF8=0]="UTF8",e[e.ASCII=1]="ASCII",e[e.Extensive=2]="Extensive",e[e.Attribute=3]="Attribute",e[e.Text=4]="Text"})(Su||(Su={}));var rv=class{constructor(e={}){ue(this,"src_Any",gi.source);ue(this,"src_Cc",pi.source);ue(this,"src_Z",mi.source);ue(this,"src_P",Pn.source);ue(this,"src_ZPCc",[this.src_Z,this.src_P,this.src_Cc].join("|"));ue(this,"src_ZCc",[this.src_Z,this.src_Cc].join("|"));ue(this,"cache",{});ue(this,"opts",{maxLength:1e4,urlAuth:!1,schema_names:[]});this.opts={...this.opts,...e}}set(e={}){return this.opts={...this.opts,...e},this.cache={},this}escapeRE(e){return e.replace(/[.?*+^$[\]\\(){}|-]/g,"\\$&")}nestedPairRE(e,t,n=4){let r=this.escapeRE(e),a=this.escapeRE(t),i=`(?:(?!${this.src_ZCc}|${r}|${a}).)`,o=`${r}${i}{0,1000}${a}`;for(let s=2;s<=n;s++)o=`${r}(?:${i}|${o}){0,1000}${a}`;return o}get_text_separators(){var e;return(e=this.cache).text_separators??(e.text_separators=/[><\uff5c]/)}get_pseudo_letter(){var e;return(e=this.cache).src_pseudo_letter??(e.src_pseudo_letter=new RegExp(`(?:(?!${this.get_text_separators().source}|${this.src_ZPCc})${this.src_Any})`))}get_ipv4_addr(){var e;return(e=this.cache).src_ip4??(e.src_ip4=new RegExp("(?:(?:25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9][0-9]|[0-9])[.]){3}(?:25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9][0-9]|[0-9])"))}get_ipv6_addr(){var n;let e="[0-9A-Fa-f]{1,4}",t=`(?:(?:${e}:${e})|${this.get_ipv4_addr().source})`;return(n=this.cache).src_ip6_addr??(n.src_ip6_addr=new RegExp(`(?:(?:${e}:){6}${t}|::(?:${e}:){5}${t}|(?:${e})?::(?:${e}:){4}${t}|(?:(?:${e}:){0,1}${e})?::(?:${e}:){3}${t}|(?:(?:${e}:){0,2}${e})?::(?:${e}:){2}${t}|(?:(?:${e}:){0,3}${e})?::${e}:${t}|(?:(?:${e}:){0,4}${e})?::${t}|(?:(?:${e}:){0,5}${e})?::${e}|(?:(?:${e}:){0,6}${e})?::)`))}get_ipv6_url_host(){var e;return(e=this.cache).src_ip6_host??(e.src_ip6_host=new RegExp(`\\[${this.get_ipv6_addr().source}\\]`))}get_ipv6_mail_host(){var e;return(e=this.cache).src_ipv6_mail_host??(e.src_ipv6_mail_host=new RegExp(`\\[IPv6:${this.get_ipv6_addr().source}\\]`))}get_auth(){var e;return(e=this.cache).src_auth??(e.src_auth=new RegExp(`(?:(?:(?!${this.src_ZCc}|[@/\\[\\]()]).){1,50}@)?`))}get_port(){var e;return(e=this.cache).src_port??(e.src_port=new RegExp("(?::(?:6(?:[0-4]\\d{3}|5(?:[0-4]\\d{2}|5(?:[0-2]\\d|3[0-5])))|[1-5]?\\d{1,4}))?"))}get_host_terminator(){var e;return(e=this.cache).src_host_terminator??(e.src_host_terminator=new RegExp(`(?=$|${this.get_text_separators().source}|${this.src_ZPCc})(?!${this.opts["---"]?"-(?!--)|":"-|"}_|:\\d|\\.-|\\.(?!$|${this.src_ZPCc}))`))}get_path_terminator(){var e;return(e=this.cache).src_path_terminator??(e.src_path_terminator=new RegExp(`${this.src_ZPCc}|${this.get_text_separators().source}`))}get_path(){var e;return(e=this.cache).src_path??(e.src_path=new RegExp(`(?:[/?#](?:${this.nestedPairRE("[","]")}|${this.nestedPairRE("(",")")}|${this.nestedPairRE("{","}")}|\\"(?:(?!${this.src_ZCc}|["]).){1,100}\\"|\\'(?:(?!${this.src_ZCc}|[']).){1,100}\\'|\\'(?=${this.get_pseudo_letter().source}|[-])|\\.{2,20}[:]?[a-zA-Z0-9%/&]|\\.(?!${this.src_ZCc}|[.]|$)|`+(this.opts["---"]?"\\-(?!--(?:[^-]|$))(?:-{0,19})|":"\\-{1,20}|")+`,(?!${this.src_ZCc}|$)|;(?!${this.src_ZCc}|$)|\\!{1,20}(?!${this.src_ZCc}|[!]|$)|\\?(?!${this.src_ZCc}|[?]|$)|`+this.get_path_extra().source+`[\\\\/:%@#&=_~*]|(?!${this.get_path_terminator().source}).){1,${this.opts.maxLength}}|\\/)?`))}get_mail_name(){var e;return(e=this.cache).src_mail_name??(e.src_mail_name=new RegExp("[-!#$%&'*+/=?^_`{|}~a-zA-Z0-9](?:[-!#$%&'*+/=?^_`{|}~a-zA-Z0-9]|[.](?=[-!#$%&'*+/=?^_`{|}~a-zA-Z0-9])){0,63}"))}get_xn(){var e;return(e=this.cache).src_xn??(e.src_xn=new RegExp("xn--[a-z0-9\\-]{1,59}"))}get_tld(){if(this.cache.tld)return this.cache.tld;let e=[...new Set(this.opts.tlds||[])].sort().reverse().join("|");return this.cache.tld=new RegExp(`${e||"$#none#$"}|${this.get_xn().source}`),this.cache.tld}get_domain_root(){var e;return(e=this.cache).src_domain_root??(e.src_domain_root=new RegExp("(?:"+this.get_xn().source+`|${this.get_pseudo_letter().source}{1,63})`))}get_domain(){var e;return(e=this.cache).src_domain??(e.src_domain=new RegExp("(?:"+this.get_xn().source+`|(?:${this.get_pseudo_letter().source})|(?:${this.get_pseudo_letter().source}(?:-|${this.get_pseudo_letter().source}){0,61}${this.get_pseudo_letter().source}))`))}get_url_host_port(){var e;return(e=this.cache).url_host_port??(e.url_host_port=new RegExp("(?:"+this.get_ipv6_url_host().source+`|(?:(?:(?:${this.get_domain().source})\\.){0,10}${this.get_domain().source}))`+this.get_port().source+this.get_host_terminator().source))}get_fuzzy_url_host_port(){var e;return(e=this.cache).fuzzy_url_host_port??(e.fuzzy_url_host_port=new RegExp("(?:"+(this.opts.fuzzyIP?this.get_ipv4_addr().source+"|":"")+`(?:(?:(?:${this.get_domain().source})\\.){1,10}(?:${this.get_tld().source})))`+this.get_host_terminator().source))}get_mail_host(){var e;return(e=this.cache).src_mail_host??(e.src_mail_host=new RegExp("(?:"+this.get_ipv6_mail_host().source+`|(?:(?:(?:${this.get_domain().source})\\.){0,4}${this.get_domain().source}))`+this.get_host_terminator().source))}get_fuzzy_mail_host(){var e;return(e=this.cache).src_fuzzy_mail_host??(e.src_fuzzy_mail_host=new RegExp("(?:"+this.get_ipv6_mail_host().source+`|(?:(?:(?:${this.get_domain().source})[.]){1,4}${this.get_domain_root().source}))`+this.get_host_terminator().source))}get_path_extra(){var e;return(e=this.cache).src_path_extra??(e.src_path_extra=new RegExp(""))}get_fuzzy_mail_host_search(){var e;return(e=this.cache).mail_fuzzy_host_search??(e.mail_fuzzy_host_search=new RegExp(`@${this.get_fuzzy_mail_host().source}`,"ig"))}get_fuzzy_link_search(){var e;return(e=this.cache).link_fuzzy_search??(e.link_fuzzy_search=new RegExp(`(^|(?![.:/\\-_@])(?:[$+<=>^\`|\uFF5C]|${this.src_ZPCc}))(?:(?![$+<=>^\`|\uFF5C])${this.get_fuzzy_url_host_port().source}${this.get_path().source})`,"ig"))}get_http_validator(){var e;return(e=this.cache).http_validator??(e.http_validator=new RegExp("\\/\\/"+(this.opts.urlAuth?this.get_auth().source:"")+this.get_url_host_port().source+this.get_path().source,"iy"))}get_relative_proto_validator(){var e;return(e=this.cache).relative_proto_validator??(e.relative_proto_validator=new RegExp((this.opts.urlAuth?this.get_auth().source:"")+`(?:localhost|${this.get_ipv6_url_host().source}|(?:(?:${this.get_domain().source})[.]){1,10}${this.get_domain_root().source})`+this.get_port().source+this.get_host_terminator().source+this.get_path().source,"iy"))}get_mail_name_validator(){var e;return(e=this.cache).mail_name_validator??(e.mail_name_validator=new RegExp(`(?:^|${this.get_text_separators().source}|"|\\(|${this.src_ZCc})(${this.get_mail_name().source})$`))}get_mailto_validator(){var e;return(e=this.cache).mailto_validator??(e.mailto_validator=new RegExp(`${this.get_mail_name().source}@${this.get_mail_host().source}`,"iy"))}get_schema_names(){var e;return(e=this.cache).schema_names??(e.schema_names=new RegExp((this.opts.schema_names||[]).map(t=>this.escapeRE(t)).join("|")))}get_schema_search(){var e;return(e=this.cache).schema_search??(e.schema_search=new RegExp(`(^|(?!_)(?:[><\uFF5C]|${this.src_ZPCc}))(${this.get_schema_names().source})`,"ig"))}get_schema_at_start(){var e;return(e=this.cache).schema_at_start??(e.schema_at_start=new RegExp(`^${this.get_schema_search().source}`,"i"))}},xi={validate:(e,t,n)=>{let r=n.re.get_http_validator();r.lastIndex=t;let a=r.exec(e);return a?a[0].length:0},normalize:(e,t)=>t.normalize(e)},av={"http:":xi,"https:":xi,"ftp:":xi,"//":{validate:function(e,t,n){let r=n.re.get_relative_proto_validator();r.lastIndex=t;let a=r.exec(e);return a?t>=3&&e[t-3]===":"||t>=3&&e[t-3]==="/"?0:a[0].length:0},normalize:(e,t)=>t.normalize(e)},"mailto:":{validate:function(e,t,n){let r=n.re.get_mailto_validator();r.lastIndex=t;let a=r.exec(e);return a?a[0].length:0},normalize:(e,t)=>t.normalize(e)}},iv="a:cdefgilmnoqrstuwxz|b:abdefghijmnorstvwyz|c:acdfghiklmnoruvwxyz|d:ejkmoz|e:cegrstu|f:ijkmor|g:abdefghilmnpqrstuwy|h:kmnrtu|i:delmnoqrst|j:emop|k:eghimnprwyz|l:abcikrstuvy|m:acdeghklmnopqrstuvwxyz|n:acefgilopruz|o:m|p:aefghklmnrstwy|q:a|r:eosuw|s:abcdeghijklmnortuvxyz|t:cdfghjklmnortvwz|u:agksyz|v:aceginu|w:fs|y:et|z:amw",ov="biz|com|edu|gov|net|org|pro|web|xxx|aero|asia|coop|info|museum|name|shop|\u0440\u0444";function sv(){let e=ov.split("|");return iv.split("|").forEach(t=>{let n=t.indexOf(":"),r=t.slice(0,n);for(let a of t.slice(n+1))e.push(r+a)}),e}var lv={fuzzyLink:!1,fuzzyEmail:!0,fuzzyIP:!1,"---":!1,tlds:sv(),urlAuth:!1,maxLength:1e4},Ru=class{constructor(e,t,n,r){ue(this,"schema");ue(this,"index");ue(this,"lastIndex");ue(this,"raw");ue(this,"text");ue(this,"url");let a=e.slice(n,r);this.schema=t.toLowerCase(),this.index=n,this.lastIndex=r,this.raw=a,this.text=a,this.url=a}},Au=class{constructor(e={}){ue(this,"__opts__");ue(this,"__schemas__");ue(this,"re");let{rebuilder:t,...n}=e;this.__opts__={...lv,...n},this.__schemas__={...av},this.re=t||new rv,this.re.set({...this.__opts__,schema_names:Object.keys(this.__schemas__)})}add(e,t=null){if(!t)delete this.__schemas__[e];else{let n={normalize:(r,a)=>a.normalize(r),...t};this.__schemas__[e]=n}return this.re.set({...this.__opts__,schema_names:Object.keys(this.__schemas__)}),this}set(e={}){return this.__opts__={...this.__opts__,...e},this.re.set({...this.__opts__,schema_names:Object.keys(this.__schemas__)}),this}test(e){if(!e.length)return!1;let t,n;for(n=this.re.get_schema_search(),n.lastIndex=0;(t=n.exec(e))!==null;)if(this.testSchemaAt(e,t[2],n.lastIndex))return!0;if(this.__opts__.fuzzyLink&&this.__schemas__["http:"]&&(n=this.re.get_fuzzy_link_search(),n.lastIndex=0,n.exec(e)!==null))return!0;if(this.__opts__.fuzzyEmail&&this.__schemas__["mailto:"]&&e.indexOf("@")>=0){let r=this.re.get_fuzzy_mail_host_search(),a=this.re.get_mail_name_validator();for(r.lastIndex=0;(t=r.exec(e))!==null;){let i=e.slice(Math.max(0,t.index-65),t.index);if(a.test(i))return!0}}return!1}testSchemaAt(e,t,n){return this.__schemas__[t.toLowerCase()]?this.__schemas__[t.toLowerCase()].validate(e.slice(0,n+this.__opts__.maxLength),n,this):0}match(e){let t=[],n=this.re.get_schema_search(),r,a,i,o,s,l,u=!1,c=!1,d=!1,f=0;if(!e.length)return null;for(n.lastIndex=0,this.__opts__.fuzzyLink&&this.__schemas__["http:"]&&(r=this.re.get_fuzzy_link_search(),r.lastIndex=0),this.__opts__.fuzzyEmail&&this.__schemas__["mailto:"]&&(a=this.re.get_fuzzy_mail_host_search(),a.lastIndex=0,i=this.re.get_mail_name_validator());;){let h=Math.max(f-1,0);if(a&&i&&!d&&(!s||s.index<f))for(a.lastIndex<h&&(a.lastIndex=h);;){let b=a.exec(e);if(!b){d=!0,s=void 0;break}let y=i.exec(e.slice(Math.max(0,b.index-65),b.index));if(y){if(s={schema:"mailto:",index:b.index-y[1].length,lastIndex:b.index+b[0].length},s.index>=f)break;a.lastIndex<h&&(a.lastIndex=h)}}if(r&&!c&&(!o||o.index<f))for(r.lastIndex<h&&(r.lastIndex=h);;){let b=r.exec(e);if(!b){c=!0,o=void 0;break}if(o={schema:"",index:b.index+b[1].length,lastIndex:b.index+b[0].length},o.index>=f)break;r.lastIndex<h&&(r.lastIndex=h)}let g=s;(!g||o&&(o.index<g.index||o.index===g.index&&o.lastIndex>g.lastIndex))&&(g=o);let m;if(!u)for(;;){if(!l){n.lastIndex<h&&(n.lastIndex=h);let _=n.exec(e);if(!_){u=!0;break}l={schema:_[2],index:_.index+_[1].length,lastIndex:_.index+_[0].length}}if(l.index<f){l=void 0;continue}if(g&&l.index>g.index)break;let b=l;l=void 0;let y=this.testSchemaAt(e,b.schema,b.lastIndex);if(y){m={schema:b.schema,index:b.index,lastIndex:b.lastIndex+y};break}}let v=m;if((!v||s&&(s.index<v.index||s.index===v.index&&s.lastIndex>v.lastIndex))&&(v=s),(!v||o&&(o.index<v.index||o.index===v.index&&o.lastIndex>v.lastIndex))&&(v=o),!v)break;v===s?s=void 0:v===o&&(o=void 0);let p=new Ru(e,v.schema,v.index,v.lastIndex);p.schema?this.__schemas__[p.schema].normalize(p,this):this.normalize(p),t.push(p),f=v.lastIndex}return t.length?t:null}matchAtStart(e){if(!e.length)return null;let t=this.re.get_schema_at_start().exec(e);if(!t)return null;let n=this.testSchemaAt(e,t[2],t[0].length);if(!n)return null;let r=new Ru(e,t[2],t.index+t[1].length,t.index+t[0].length+n);return this.__schemas__[r.schema].normalize(r,this),r}tlds(e,t=!1){return e=Array.isArray(e)?e:[e],t?this.__opts__.tlds=this.__opts__.tlds.concat(e):this.__opts__.tlds=e,this.re.set({...this.__opts__,schema_names:Object.keys(this.__schemas__)}),this}normalize(e){e.schema||(e.url=`http://${e.url}`),e.schema==="mailto:"&&!/^mailto:/i.test(e.url)&&(e.url=`mailto:${e.url}`)}};var uv=/^xn--/,cv=/[^\0-\x7F]/,dv=/[\x2E\u3002\uFF0E\uFF61]/g,hv={overflow:"Overflow: input needs wider integers to process","not-basic":"Illegal input >= 0x80 (not a basic code point)","invalid-input":"Invalid input"},Ti=35,He=Math.floor,Ei=String.fromCharCode;function lt(e){throw new RangeError(hv[e])}function fv(e,t){let n=[],r=e.length;for(;r--;)n[r]=t(e[r]);return n}function Du(e,t){let n=e.split("@"),r="";n.length>1&&(r=n[0]+"@",e=n[1]),e=e.replace(dv,".");let a=e.split("."),i=fv(a,t).join(".");return r+i}function Cu(e){let t=[],n=0,r=e.length;for(;n<r;){let a=e.charCodeAt(n++);if(a>=55296&&a<=56319&&n<r){let i=e.charCodeAt(n++);(i&64512)==56320?t.push(((a&1023)<<10)+(i&1023)+65536):(t.push(a),n--)}else t.push(a)}return t}var gv=e=>String.fromCodePoint(...e),pv=function(e){return e>=48&&e<58?26+(e-48):e>=65&&e<91?e-65:e>=97&&e<123?e-97:36},wu=function(e,t){return e+22+75*(e<26)-((t!=0)<<5)},ku=function(e,t,n){let r=0;for(e=n?He(e/700):e>>1,e+=He(e/t);e>Ti*26>>1;r+=36)e=He(e/Ti);return He(r+(Ti+1)*e/(e+38))},Lu=function(e){let t=[],n=e.length,r=0,a=128,i=72,o=e.lastIndexOf("-");o<0&&(o=0);for(let s=0;s<o;++s)e.charCodeAt(s)>=128&&lt("not-basic"),t.push(e.charCodeAt(s));for(let s=o>0?o+1:0;s<n;){let l=r;for(let c=1,d=36;;d+=36){s>=n&&lt("invalid-input");let f=pv(e.charCodeAt(s++));f>=36&&lt("invalid-input"),f>He((2147483647-r)/c)&&lt("overflow"),r+=f*c;let h=d<=i?1:d>=i+26?26:d-i;if(f<h)break;let g=36-h;c>He(2147483647/g)&&lt("overflow"),c*=g}let u=t.length+1;i=ku(r-l,u,l==0),He(r/u)>2147483647-a&&lt("overflow"),a+=He(r/u),r%=u,t.splice(r++,0,a)}return String.fromCodePoint(...t)},Fu=function(e){let t=[];e=Cu(e);let n=e.length,r=128,a=0,i=72;for(let l of e)l<128&&t.push(Ei(l));let o=t.length,s=o;for(o&&t.push("-");s<n;){let l=2147483647;for(let c of e)c>=r&&c<l&&(l=c);let u=s+1;l-r>He((2147483647-a)/u)&&lt("overflow"),a+=(l-r)*u,r=l;for(let c of e)if(c<r&&++a>2147483647&&lt("overflow"),c===r){let d=a;for(let f=36;;f+=36){let h=f<=i?1:f>=i+26?26:f-i;if(d<h)break;let g=d-h,m=36-h;t.push(Ei(wu(h+g%m,0))),d=He(g/m)}t.push(Ei(wu(d,0))),i=ku(a,u,s===o),a=0,++s}++a,++r}return t.join("")},vv=function(e){return Du(e,function(t){return uv.test(t)?Lu(t.slice(4).toLowerCase()):t})},mv=function(e){return Du(e,function(t){return cv.test(t)?"xn--"+Fu(t):t})},bv={version:"2.3.1",ucs2:{decode:Cu,encode:gv},decode:Lu,encode:Fu,toASCII:mv,toUnicode:vv};var Si=bv;var Pu=Object.defineProperty,$u=(e,t)=>{let n={};for(var r in e)Pu(n,r,{get:e[r],enumerable:!0});return t||Pu(n,Symbol.toStringTag,{value:"Module"}),n},yv=$u({arrayReplaceAt:()=>_v,asciiTrim:()=>ra,callable:()=>Vu,escapeHtml:()=>ut,escapeRE:()=>kv,fromCodePoint:()=>Nn,isMdAsciiPunct:()=>Gn,isPunctChar:()=>Xu,isPunctCharCode:()=>Mn,isSpace:()=>ne,isValidEntityCode:()=>ki,isWhiteSpace:()=>zn,lib:()=>Lv,normalizeReference:()=>na,unescapeAll:()=>Jt,unescapeMd:()=>Sv});function Vu(e){let t=function(...n){return Reflect.construct(e,n,new.target&&new.target!==t?new.target:e)};return Object.defineProperty(t,"name",{value:e.name}),Object.setPrototypeOf(t,e),t.prototype=e.prototype,t}function _v(e,t,n){return[].concat(e.slice(0,t),n,e.slice(t+1))}function ki(e){return!(e>=55296&&e<=57343||e>=64976&&e<=65007||(e&65535)===65535||(e&65535)===65534||e>=0&&e<=8||e===11||e>=14&&e<=31||e>=127&&e<=159||e>1114111)}function Nn(e){if(e>65535){e-=65536;let t=55296+(e>>10),n=56320+(e&1023);return String.fromCharCode(t,n)}return String.fromCharCode(e)}var qu=/\\([!"#$%&'()*+,\-./:;<=>?@[\\\]^_`{|}~])/g,xv=new RegExp(`${qu.source}|${/&([a-z#][a-z0-9]{1,31});/gi.source}`,"gi"),Tv=/^#((?:x[a-f0-9]{1,8}|[0-9]{1,8}))$/i;function Ev(e,t){if(t.charCodeAt(0)===35&&Tv.test(t)){let r=t[1].toLowerCase()==="x"?parseInt(t.slice(2),16):parseInt(t.slice(1),10);return ki(r)?Nn(r):e}let n=Jr(e);return n!==e?n:e}function Sv(e){return e.indexOf("\\")<0?e:e.replace(qu,"$1")}function Jt(e){return e.indexOf("\\")<0&&e.indexOf("&")<0?e:e.replace(xv,function(t,n,r){return n||Ev(t,r)})}var Rv=/[&<>"]/,Av=/[&<>"]/g,wv={"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;"};function Dv(e){return wv[e]}function ut(e){return Rv.test(e)?e.replace(Av,Dv):e}var Cv=/[.?*+^$[\]\\(){}|-]/g;function kv(e){return e.replace(Cv,"\\$&")}function ne(e){switch(e){case 9:case 32:return!0}return!1}function zn(e){if(e>=8192&&e<=8202)return!0;switch(e){case 9:case 10:case 11:case 12:case 13:case 32:case 160:case 5760:case 8239:case 8287:case 12288:return!0}return!1}function Xu(e){return Pn.test(e)||vi.test(e)}function Mn(e){return Xu(Nn(e))}function Gn(e){switch(e){case 33:case 34:case 35:case 36:case 37:case 38:case 39:case 40:case 41:case 42:case 43:case 44:case 45:case 46:case 47:case 58:case 59:case 60:case 61:case 62:case 63:case 64:case 91:case 92:case 93:case 94:case 95:case 96:case 123:case 124:case 125:case 126:return!0;default:return!1}}function na(e){return e=e.trim().replace(/\s+/g," "),e.toLowerCase().toUpperCase()}function Iu(e){return e===32||e===9||e===10||e===13}function ra(e){let t=0;for(;t<e.length&&Iu(e.charCodeAt(t));t++);let n=e.length-1;for(;n>=t&&Iu(e.charCodeAt(n));n--);return e.slice(t,n+1)}var Lv={mdurl:fi,ucmicro:bi};function Fv(e,t,n){let r,a,i,o,s=e.posMax,l=e.pos;for(e.pos=t+1,r=1;e.pos<s;){if(i=e.src.charCodeAt(e.pos),i===93&&(r--,r===0)){a=!0;break}if(o=e.pos,e.md.inline.skipToken(e),i===91){if(o===e.pos-1)r++;else if(n)return e.pos=l,-1}}let u=-1;return a&&(u=e.pos),e.pos=l,u}function Pv(e,t,n){let r,a=t,i={ok:!1,pos:0,str:""};if(e.charCodeAt(a)===60){for(a++;a<n;){if(r=e.charCodeAt(a),r===10||r===60)return i;if(r===62)return i.pos=a+1,i.str=Jt(e.slice(t+1,a)),i.ok=!0,i;if(r===92&&a+1<n){a+=2;continue}a++}return i}let o=0;for(;a<n&&(r=e.charCodeAt(a),!(r===32||r<32||r===127));){if(r===92&&a+1<n){if(e.charCodeAt(a+1)===32){a++;continue}a+=2;continue}if(r===40&&(o++,o>32))return i;if(r===41){if(o===0)break;o--}a++}return t===a||o!==0||(i.str=Jt(e.slice(t,a)),i.pos=a,i.ok=!0),i}function Iv(e,t,n,r){let a,i=t,o={ok:!1,can_continue:!1,pos:0,str:"",marker:0};if(r)o.str=r.str,o.marker=r.marker;else{if(i>=n)return o;let s=e.charCodeAt(i);if(s!==34&&s!==39&&s!==40)return o;t++,i++,s===40&&(s=41),o.marker=s}for(;i<n;){if(a=e.charCodeAt(i),a===o.marker)return o.pos=i+1,o.str+=Jt(e.slice(t,i)),o.ok=!0,o;if(a===40&&o.marker===41)return o;a===92&&i+1<n&&i++,i++}return o.can_continue=!0,o.str+=Jt(e.slice(t,i)),o}var Nv=$u({parseLinkDestination:()=>Pv,parseLinkLabel:()=>Fv,parseLinkTitle:()=>Iv});function Bn(e){"@babel/helpers - typeof";return Bn=typeof Symbol=="function"&&typeof Symbol.iterator=="symbol"?function(t){return typeof t}:function(t){return t&&typeof Symbol=="function"&&t.constructor===Symbol&&t!==Symbol.prototype?"symbol":typeof t},Bn(e)}function zv(e,t){if(Bn(e)!="object"||!e)return e;var n=e[Symbol.toPrimitive];if(n!==void 0){var r=n.call(e,t||"default");if(Bn(r)!="object")return r;throw new TypeError("@@toPrimitive must return a primitive value.")}return(t==="string"?String:Number)(e)}function Mv(e){var t=zv(e,"string");return Bn(t)=="symbol"?t:t+""}function H(e,t,n){return(t=Mv(t))in e?Object.defineProperty(e,t,{value:n,enumerable:!0,configurable:!0,writable:!0}):e[t]=n,e}var At=class{constructor(e,t,n){H(this,"map",null),H(this,"level",0),H(this,"children",null),H(this,"content",""),H(this,"markup",""),H(this,"info",""),H(this,"block",!1),H(this,"hidden",!1),this.type=e,this.tag=t,this.attrs=null,this.nesting=n,this.meta=null}attrIndex(e){if(!this.attrs)return-1;let t=this.attrs;for(let n=0,r=t.length;n<r;n++)if(t[n][0]===e)return n;return-1}attrPush(e){this.attrs?this.attrs.push(e):this.attrs=[e]}attrSet(e,t){let n=this.attrIndex(e),r=[e,t];n<0?this.attrPush(r):this.attrs[n]=r}attrGet(e){let t=this.attrIndex(e),n=null;return t>=0&&(n=this.attrs[t][1]),n}attrJoin(e,t){let n=this.attrIndex(e);n<0?this.attrPush([e,t]):this.attrs[n][1]=`${this.attrs[n][1]} ${t}`}},Wn=class{constructor(){H(this,"__rules__",[]),H(this,"__cache__",null)}__find__(e){for(let t=0;t<this.__rules__.length;t++)if(this.__rules__[t].name===e)return t;return-1}__compile__(){let e=new Set;this.__rules__.forEach(t=>{t.enabled&&t.alt.forEach(n=>{n&&e.add(n)})}),this.__cache__=Object.create(null),this.__cache__[""]=[],this.__rules__.forEach(t=>{t.enabled&&this.__cache__[""].push(t.fn)}),e.forEach(t=>{this.__cache__[t]=[],this.__rules__.forEach(n=>{n.enabled&&n.alt.indexOf(t)>=0&&this.__cache__[t].push(n.fn)})})}at(e,t,n={}){let r=this.__find__(e);if(r===-1)throw new Error(`Parser rule not found: ${e}`);this.__rules__[r].fn=t,this.__rules__[r].alt=n.alt||[],this.__cache__=null}before(e,t,n,r={}){let a=this.__find__(e);if(a===-1)throw new Error(`Parser rule not found: ${e}`);this.__rules__.splice(a,0,{name:t,enabled:!0,fn:n,alt:r.alt||[]}),this.__cache__=null}after(e,t,n,r={}){let a=this.__find__(e);if(a===-1)throw new Error(`Parser rule not found: ${e}`);this.__rules__.splice(a+1,0,{name:t,enabled:!0,fn:n,alt:r.alt||[]}),this.__cache__=null}push(e,t,n={}){this.__rules__.push({name:e,enabled:!0,fn:t,alt:n.alt||[]}),this.__cache__=null}enable(e,t=!1){Array.isArray(e)||(e=[e]);let n=[];return e.forEach(r=>{let a=this.__find__(r);if(a<0){if(t)return;throw new Error(`Rules manager: invalid rule name ${r}`)}this.__rules__[a].enabled=!0,n.push(r)}),this.__cache__=null,n}enableOnly(e,t=!1){Array.isArray(e)||(e=[e]),this.__rules__.forEach(n=>{n.enabled=!1}),this.enable(e,t)}disable(e,t=!1){Array.isArray(e)||(e=[e]);let n=[];return e.forEach(r=>{let a=this.__find__(r);if(a<0){if(t)return;throw new Error(`Rules manager: invalid rule name ${r}`)}this.__rules__[a].enabled=!1,n.push(r)}),this.__cache__=null,n}getRules(e){return this.__cache__||this.__compile__(),this.__cache__[e]||[]}},je={};je.code_inline=function(e,t,n,r,a){let i=e[t];return`<code${a.renderAttrs(i)}>${ut(i.content)}</code>`};je.code_block=function(e,t,n,r,a){let i=e[t];return`<pre${a.renderAttrs(i)}><code>${ut(e[t].content)}</code></pre>
`};je.fence=function(e,t,n,r,a){let i=e[t],o=i.info?Jt(i.info).trim():"",s="",l="";if(o){let c=o.split(/(\s+)/g);s=c[0],l=c.slice(2).join("")}let u;if(n.highlight?u=n.highlight(i.content,s,l)||ut(i.content):u=ut(i.content),u.indexOf("<pre")===0)return u+`
`;if(o){let c=i.attrIndex("class"),d=i.attrs?i.attrs.slice():[];c<0?d.push(["class",`${n.langPrefix}${s}`]):(d[c]=[d[c][0],d[c][1]],d[c][1]+=` ${n.langPrefix}${s}`);let f={attrs:d};return`<pre><code${a.renderAttrs(f)}>${u}</code></pre>
`}return`<pre><code${a.renderAttrs(i)}>${u}</code></pre>
`};je.image=function(e,t,n,r,a){let i=e[t];return i.attrs[i.attrIndex("alt")][1]=a.renderInlineAsText(i.children,n,r),a.renderToken(e,t,n)};je.hardbreak=function(e,t,n){return n.xhtmlOut?`<br />
`:`<br>
`};je.softbreak=function(e,t,n){return n.breaks?n.xhtmlOut?`<br />
`:`<br>
`:`
`};je.text=function(e,t){return ut(e[t].content)};je.html_block=function(e,t){return e[t].content};je.html_inline=function(e,t){return e[t].content};var Yu=class{constructor(){H(this,"rules",Object.assign({},je))}renderAttrs(e){let t,n,r;if(!e.attrs)return"";for(r="",t=0,n=e.attrs.length;t<n;t++)r+=` ${ut(e.attrs[t][0])}="${ut(String(e.attrs[t][1]))}"`;return r}renderToken(e,t,n){let r=e[t],a="";if(r.hidden)return"";let i=t-1;for(;i>=0&&e[i].hidden&&e[i].nesting===0;)i--;r.block&&r.nesting!==-1&&i>=0&&e[i].hidden&&e[i].nesting===-1&&(a+=`
`),a+=(r.nesting===-1?"</":"<")+r.tag,a+=this.renderAttrs(r),r.nesting===0&&n.xhtmlOut&&(a+=" /");let o=!1;if(r.block&&(o=!0,r.nesting===1)){let s=t+1;for(;s<e.length&&e[s].hidden&&e[s].nesting===0;)s++;if(s<e.length){let l=e[s];(l.type==="inline"||l.hidden||l.nesting===-1&&l.tag===r.tag)&&(o=!1)}}return a+=o?`>
`:">",a}renderInline(e,t,n){let r="",a=this.rules;for(let i=0,o=e.length;i<o;i++){let s=e[i].type;typeof a[s]<"u"?r+=a[s](e,i,t,n,this):r+=this.renderToken(e,i,t)}return r}renderInlineAsText(e,t,n){let r="";for(let a=0,i=e.length;a<i;a++)switch(e[a].type){case"text":case"code_inline":r+=e[a].content;break;case"image":r+=this.renderInlineAsText(e[a].children,t,n);break;case"html_inline":case"html_block":r+=e[a].content;break;case"softbreak":case"hardbreak":r+=`
`}return r}render(e,t,n){let r="",a=this.rules;for(let i=0,o=e.length;i<o;i++){let s=e[i].type;s==="inline"?r+=this.renderInline(e[i].children,t,n):typeof a[s]<"u"?r+=a[s](e,i,t,n,this):r+=this.renderToken(e,i,t)}return r}},Ku=class{constructor(e,t,n){H(this,"tokens",[]),H(this,"inlineMode",!1),H(this,"Token",At),this.src=e,this.env=n,this.md=t}},Gv=/\r\n?/g,Bv=/\0/g;function Wv(e){let t;t=e.src.replace(Gv,`
`),t=t.replace(Bv,"\uFFFD"),e.src=t}function Ov(e){let t;e.inlineMode?(t=new e.Token("inline","",0),t.content=e.src,t.map=[0,1],t.children=[],e.tokens.push(t)):e.md.block.parse(e.src,e.md,e.env,e.tokens)}function Uv(e){let t=e.tokens,n=0;for(let r=0;r<t.length;r++)t[r].type!=="reference_definition"&&(r!==n&&(t[n]=t[r]),n++);t.length!==n&&(t.length=n)}function Hv(e){let t=e.tokens;for(let n=0,r=t.length;n<r;n++){let a=t[n];a.type==="inline"&&e.md.inline.parse(a.content,e.md,e.env,a.children)}}function jv(e){return/^<a[>\s]/i.test(e)}function $v(e){return/^<\/a\s*>/i.test(e)}function Vv(e){let t=e.tokens;if(e.md.options.linkify)for(let n=0,r=t.length;n<r;n++){if(t[n].type!=="inline"||!e.md.linkify.test(t[n].content))continue;let a=t[n].children,i=[],o=0;for(let s=a.length-1;s>=0;s--){let l=a[s];if(l.type==="link_close"){for(s--;a[s].level!==l.level&&a[s].type!=="link_open";)s--;continue}if(l.type==="html_inline"&&(jv(l.content)&&o>0&&o--,$v(l.content)&&o++),!(o>0)&&l.type==="text"&&e.md.linkify.test(l.content)){let u=l.content,c=e.md.linkify.match(u),d=[],f=l.level,h=0;c.length>0&&c[0].index===0&&s>0&&a[s-1].type==="text_special"&&(c=c.slice(1));for(let g=0;g<c.length;g++){let m=c[g].url,v=e.md.normalizeLink(m);if(!e.md.validateLink(v))continue;let p=c[g].text;c[g].schema?c[g].schema==="mailto:"&&!/^mailto:/i.test(p)?p=e.md.normalizeLinkText(`mailto:${p}`).replace(/^mailto:/,""):p=e.md.normalizeLinkText(p):p=e.md.normalizeLinkText(`http://${p}`).replace(/^http:\/\//,"");let b=c[g].index;if(b>h){let T=new e.Token("text","",0);T.content=u.slice(h,b),T.level=f,d.push(T)}let y=new e.Token("link_open","a",1);y.attrs=[["href",v]],y.level=f++,y.markup="linkify",y.info="auto",d.push(y);let _=new e.Token("text","",0);_.content=p,_.level=f,d.push(_);let x=new e.Token("link_close","a",-1);x.level=--f,x.markup="linkify",x.info="auto",d.push(x),h=c[g].lastIndex}if(h<u.length){let g=new e.Token("text","",0);g.content=u.slice(h),g.level=f,d.push(g)}i.push({index:s,nodes:d})}}if(i.length>0){let s=a.length;for(let d of i)s+=d.nodes.length-1;let l=new Array(s),u=0,c=0;i.reverse();for(let d=0;d<a.length;d++){let f=i[u];if(f?.index===d){for(let h of f.nodes)l[c++]=h;u++}else l[c++]=a[d]}t[n].children=l}}}var Zu=/\+-|\.\.|\?\?\?\?|!!!!|,,|--/,qv=/\((c|tm|r)\)/i,Xv=/\((c|tm|r)\)/gi,Yv={c:"\xA9",r:"\xAE",tm:"\u2122"};function Kv(e,t){return Yv[t.toLowerCase()]}function Zv(e){let t=0;for(let n=e.length-1;n>=0;n--){let r=e[n];r.type==="text"&&!t&&(r.content=r.content.replace(Xv,Kv)),r.type==="link_open"&&r.info==="auto"&&t--,r.type==="link_close"&&r.info==="auto"&&t++}}function Jv(e){let t=0;for(let n=e.length-1;n>=0;n--){let r=e[n];r.type==="text"&&!t&&Zu.test(r.content)&&(r.content=r.content.replace(/\+-/g,"\xB1").replace(/\.{2,}/g,"\u2026").replace(/([?!])…/g,"$1..").replace(/([?!]){4,}/g,"$1$1$1").replace(/,{2,}/g,",").replace(/(^|[^-])---(?=[^-]|$)/gm,"$1\u2014").replace(/(^|\s)--(?=\s|$)/gm,"$1\u2013").replace(/(^|[^-\s])--(?=[^-\s]|$)/gm,"$1\u2013")),r.type==="link_open"&&r.info==="auto"&&t--,r.type==="link_close"&&r.info==="auto"&&t++}}function Qv(e){let t;if(e.md.options.typographer)for(t=e.tokens.length-1;t>=0;t--)e.tokens[t].type==="inline"&&(qv.test(e.tokens[t].content)&&Zv(e.tokens[t].children),Zu.test(e.tokens[t].content)&&Jv(e.tokens[t].children))}var em=/['"]/,Nu=/['"]/g,zu="\u2019",tm=1e3;function Mu(e,t,n){for(;e.length>n;){let r=e.pop();r.isSingleQuote?t.single=r.prevSameQuoteIdx:t.double=r.prevSameQuoteIdx}}function Qr(e,t,n,r){e[t]||(e[t]=[]),e[t].push({pos:n,ch:r})}function nm(e,t){let n="",r=0;t.sort((a,i)=>a.pos-i.pos);for(let a=0;a<t.length;a++){let i=t[a];n+=e.slice(r,i.pos)+i.ch,r=i.pos+1}return n+e.slice(r)}function rm(e,t){let n,r=[],a={single:-1,double:-1},i={};for(let o=0;o<e.length;o++){let s=e[o],l=e[o].level;for(n=r.length-1;n>=0&&!(r[n].level<=l);n--);if(Mu(r,a,n+1),s.type!=="text")continue;let u=s.content,c=0,d=u.length;e:for(;c<d;){Nu.lastIndex=c;let f=Nu.exec(u);if(!f)break;let h=!0,g=!0;c=f.index+1;let m=f[0]==="'",v=32;if(f.index-1>=0)v=u.charCodeAt(f.index-1);else for(n=o-1;n>=0&&!(e[n].type==="softbreak"||e[n].type==="hardbreak");n--)if(e[n].content){v=e[n].content.charCodeAt(e[n].content.length-1);break}let p=32;if(c<d)p=u.charCodeAt(c);else for(n=o+1;n<e.length&&!(e[n].type==="softbreak"||e[n].type==="hardbreak");n++)if(e[n].content){p=e[n].content.charCodeAt(0);break}let b=Gn(v)||Mn(v),y=Gn(p)||Mn(p),_=zn(v),x=zn(p);if(x?h=!1:y&&(_||b||(h=!1)),_?g=!1:b&&(x||y||(g=!1)),p===34&&f[0]==='"'&&v>=48&&v<=57&&(g=h=!1),h&&g&&(h=b,g=y),!h&&!g){m&&Qr(i,o,f.index,zu);continue}if(g&&(n=m?a.single:a.double,n>=0&&r[n].level===l)){let T=r[n],E,S;m?(E=t.md.options.quotes[2],S=t.md.options.quotes[3]):(E=t.md.options.quotes[0],S=t.md.options.quotes[1]),Qr(i,o,f.index,S),Qr(i,T.tokenIdx,T.contentPos,E),Mu(r,a,n);continue e}if(h){if(r.length>=tm)return;r.push({tokenIdx:o,contentPos:f.index,isSingleQuote:m,level:l,prevSameQuoteIdx:m?a.single:a.double}),m?a.single=r.length-1:a.double=r.length-1}else g&&m&&Qr(i,o,f.index,zu)}}Object.keys(i).forEach(function(o){let s=Number(o);e[s].content=nm(e[s].content,i[o])})}function am(e){if(e.md.options.typographer)for(let t=e.tokens.length-1;t>=0;t--)e.tokens[t].type!=="inline"||!em.test(e.tokens[t].content)||rm(e.tokens[t].children,e)}function im(e){let t,n,r=e.length;for(t=0;t<r;t++)e[t].type==="text_special"&&(e[t].type="text");for(t=n=0;t<r;t++)e[t].type==="text"&&t+1<r&&e[t+1].type==="text"?e[t+1].content=e[t].content+e[t+1].content:(t!==n&&(e[n]=e[t]),n++);t!==n&&(e.length=n)}function om(e){let t,n,r=e.tokens,a=r.length;for(let i=0;i<a;i++){if(r[i].type!=="inline")continue;let o=r[i].children,s=o.length;for(t=0;t<s;t++)o[t].type==="text_special"&&(o[t].type="text"),o[t].children&&im(o[t].children);for(t=n=0;t<s;t++)o[t].type==="text"&&t+1<s&&o[t+1].type==="text"?o[t+1].content=o[t].content+o[t+1].content:(t!==n&&(o[n]=o[t]),n++);t!==n&&(o.length=n)}}var Ri=[["normalize",Wv],["block",Ov],["strip_references",Uv],["inline",Hv],["linkify",Vv],["replacements",Qv],["smartquotes",am],["text_join",om]],Ju=class{constructor(){H(this,"ruler",new Wn),H(this,"State",Ku);for(let e=0;e<Ri.length;e++)this.ruler.push(Ri[e][0],Ri[e][1])}process(e){let t=this.ruler.getRules("");for(let n=0,r=t.length;n<r;n++)t[n](e)}},Qu=class{constructor(e,t,n,r){H(this,"bMarks",[]),H(this,"eMarks",[]),H(this,"tShift",[]),H(this,"sCount",[]),H(this,"bsCount",[]),H(this,"blkIndent",0),H(this,"line",0),H(this,"lineMax",0),H(this,"tight",!1),H(this,"listIndent",-1),H(this,"parentType","root"),H(this,"level",0),H(this,"Token",At),this.src=e,this.md=t,this.env=n,this.tokens=r;let a=this.src;for(let i=0,o=0,s=0,l=0,u=a.length,c=!1;o<u;o++){let d=a.charCodeAt(o);if(!c)if(ne(d)){s++,d===9?l+=4-l%4:l++;continue}else c=!0;(d===10||o===u-1)&&(d!==10&&o++,this.bMarks.push(i),this.eMarks.push(o),this.tShift.push(s),this.sCount.push(l),this.bsCount.push(0),c=!1,s=0,l=0,i=o+1)}this.bMarks.push(a.length),this.eMarks.push(a.length),this.tShift.push(0),this.sCount.push(0),this.bsCount.push(0),this.lineMax=this.bMarks.length-1}push(e,t,n){let r=new At(e,t,n);return r.block=!0,n<0&&this.level--,r.level=this.level,n>0&&this.level++,this.tokens.push(r),r}isEmpty(e){return this.bMarks[e]+this.tShift[e]>=this.eMarks[e]}skipEmptyLines(e){for(let t=this.lineMax;e<t&&!(this.bMarks[e]+this.tShift[e]<this.eMarks[e]);e++);return e}skipSpaces(e){for(let t=this.src.length;e<t&&ne(this.src.charCodeAt(e));e++);return e}skipSpacesBack(e,t){if(e<=t)return e;for(;e>t;)if(!ne(this.src.charCodeAt(--e)))return e+1;return e}skipChars(e,t){for(let n=this.src.length;e<n&&this.src.charCodeAt(e)===t;e++);return e}skipCharsBack(e,t,n){if(e<=n)return e;for(;e>n;)if(t!==this.src.charCodeAt(--e))return e+1;return e}getLines(e,t,n,r){if(e>=t)return"";let a=new Array(t-e);for(let i=0,o=e;o<t;o++,i++){let s=0,l=this.bMarks[o],u=l,c;for(o+1<t||r?c=this.eMarks[o]+1:c=this.eMarks[o];u<c&&s<n;){let d=this.src.charCodeAt(u);if(ne(d))d===9?s+=4-(s+this.bsCount[o])%4:s++;else if(u-l<this.tShift[o])s++;else break;u++}s>n?a[i]=new Array(s-n+1).join(" ")+this.src.slice(u,c):a[i]=this.src.slice(u,c)}return a.join("")}},sm=65536;function Ai(e,t){let n=e.bMarks[t]+e.tShift[t],r=e.eMarks[t];return e.src.slice(n,r)}function Gu(e){let t=[],n=e.length,r=0,a=e.charCodeAt(r),i=!1,o=0,s="";for(;r<n;)a===124&&(i?(s+=e.substring(o,r-1),o=r):(t.push(s+e.substring(o,r)),s="",o=r+1)),i=a===92,r++,a=e.charCodeAt(r);return t.push(s+e.substring(o)),t}function lm(e,t,n,r){if(t+2>n)return!1;let a=t+1;if(e.sCount[a]<e.blkIndent||e.sCount[a]-e.blkIndent>=4)return!1;let i=e.bMarks[a]+e.tShift[a];if(i>=e.eMarks[a])return!1;let o=e.src.charCodeAt(i++);if(o!==124&&o!==45&&o!==58||i>=e.eMarks[a])return!1;let s=e.src.charCodeAt(i++);if(s!==124&&s!==45&&s!==58&&!ne(s)||o===45&&ne(s))return!1;for(;i<e.eMarks[a];){let _=e.src.charCodeAt(i);if(_!==124&&_!==45&&_!==58&&!ne(_))return!1;i++}let l=Ai(e,t+1),u=l.split("|"),c=[];for(let _=0;_<u.length;_++){let x=u[_].trim();if(!x){if(_===0||_===u.length-1)continue;return!1}if(!/^:?-+:?$/.test(x))return!1;x.charCodeAt(x.length-1)===58?c.push(x.charCodeAt(0)===58?"center":"right"):x.charCodeAt(0)===58?c.push("left"):c.push("")}if(l=Ai(e,t).trim(),l.indexOf("|")===-1||e.sCount[t]-e.blkIndent>=4)return!1;u=Gu(l),u.length&&u[0]===""&&u.shift(),u.length&&u[u.length-1]===""&&u.pop();let d=u.length;if(d===0||d!==c.length)return!1;if(r)return!0;let f=e.parentType;e.parentType="table";let h=e.md.block.ruler.getRules("blockquote"),g=e.push("table_open","table",1),m=[t,0];g.map=m;let v=e.push("thead_open","thead",1);v.map=[t,t+1];let p=e.push("tr_open","tr",1);p.map=[t,t+1];for(let _=0;_<u.length;_++){let x=e.push("th_open","th",1);c[_]&&(x.attrs=[["style",`text-align:${c[_]}`]]);let T=e.push("inline","",0);T.content=u[_].trim(),T.children=[],e.push("th_close","th",-1)}e.push("tr_close","tr",-1),e.push("thead_close","thead",-1);let b,y=0;for(a=t+2;a<n&&!(e.sCount[a]<e.blkIndent);a++){let _=!1;for(let T=0,E=h.length;T<E;T++)if(h[T](e,a,n,!0)){_=!0;break}if(_||(l=Ai(e,a).trim(),!l)||e.sCount[a]-e.blkIndent>=4||(u=Gu(l),u.length&&u[0]===""&&u.shift(),u.length&&u[u.length-1]===""&&u.pop(),y+=d-u.length,y>sm))break;if(a===t+2){let T=e.push("tbody_open","tbody",1);T.map=b=[t+2,0]}let x=e.push("tr_open","tr",1);x.map=[a,a+1];for(let T=0;T<d;T++){let E=e.push("td_open","td",1);c[T]&&(E.attrs=[["style",`text-align:${c[T]}`]]);let S=e.push("inline","",0);S.content=u[T]?u[T].trim():"",S.children=[],e.push("td_close","td",-1)}e.push("tr_close","tr",-1)}return b&&(e.push("tbody_close","tbody",-1),b[1]=a),e.push("table_close","table",-1),m[1]=a,e.parentType=f,e.line=a,!0}function um(e,t,n){if(e.sCount[t]-e.blkIndent<4)return!1;let r=t+1,a=r;for(;r<n;){if(e.isEmpty(r)){r++;continue}if(e.sCount[r]-e.blkIndent>=4){r++,a=r;continue}break}e.line=a;let i=e.push("code_block","code",0);return i.content=e.getLines(t,a,4+e.blkIndent,!1)+`
`,i.map=[t,e.line],!0}function cm(e,t,n,r){let a=e.bMarks[t]+e.tShift[t],i=e.eMarks[t];if(e.sCount[t]-e.blkIndent>=4||a+3>i)return!1;let o=e.src.charCodeAt(a);if(o!==126&&o!==96)return!1;let s=a;a=e.skipChars(a,o);let l=a-s;if(l<3)return!1;let u=e.src.slice(s,a),c=e.src.slice(a,i);if(o===96&&c.indexOf(String.fromCharCode(o))>=0)return!1;if(r)return!0;let d=t,f=!1;for(;d++,!(d>=n||(a=s=e.bMarks[d]+e.tShift[d],i=e.eMarks[d],a<i&&e.sCount[d]<e.blkIndent));)if(e.src.charCodeAt(a)===o&&!(e.sCount[d]-e.blkIndent>=4)&&(a=e.skipChars(a,o),!(a-s<l)&&(a=e.skipSpaces(a),!(a<i)))){f=!0;break}l=e.sCount[t],e.line=d+(f?1:0);let h=e.push("fence","code",0);return h.info=c,h.content=e.getLines(t+1,d,l,!0),h.markup=u,h.map=[t,e.line],!0}function dm(e,t,n,r){let a=e.bMarks[t]+e.tShift[t],i=e.eMarks[t],o=e.lineMax;if(e.sCount[t]-e.blkIndent>=4||e.src.charCodeAt(a)!==62)return!1;if(r)return!0;let s=[],l=[],u=[],c=[],d=e.md.block.ruler.getRules("blockquote"),f=e.parentType;e.parentType="blockquote";let h=!1,g;for(g=t;g<n;g++){let y=e.sCount[g]<e.blkIndent;if(a=e.bMarks[g]+e.tShift[g],i=e.eMarks[g],a>=i)break;if(e.src.charCodeAt(a++)===62&&!y){let x=e.sCount[g]+1,T,E;e.src.charCodeAt(a)===32?(a++,x++,E=!1,T=!0):e.src.charCodeAt(a)===9?(T=!0,(e.bsCount[g]+x)%4===3?(a++,x++,E=!1):E=!0):T=!1;let S=x;for(s.push(e.bMarks[g]),e.bMarks[g]=a;a<i;){let w=e.src.charCodeAt(a);if(ne(w))w===9?S+=4-(S+e.bsCount[g]+(E?1:0))%4:S++;else break;a++}h=a>=i,l.push(e.bsCount[g]),e.bsCount[g]=e.sCount[g]+1+(T?1:0),u.push(e.sCount[g]),e.sCount[g]=S-x,c.push(e.tShift[g]),e.tShift[g]=a-e.bMarks[g];continue}if(h)break;let _=!1;for(let x=0,T=d.length;x<T;x++)if(d[x](e,g,n,!0)){_=!0;break}if(_){e.lineMax=g,e.blkIndent!==0&&(s.push(e.bMarks[g]),l.push(e.bsCount[g]),c.push(e.tShift[g]),u.push(e.sCount[g]),e.sCount[g]-=e.blkIndent);break}s.push(e.bMarks[g]),l.push(e.bsCount[g]),c.push(e.tShift[g]),u.push(e.sCount[g]),e.sCount[g]=-1}let m=e.blkIndent;e.blkIndent=0;let v=e.push("blockquote_open","blockquote",1);v.markup=">";let p=[t,0];v.map=p,e.md.block.tokenize(e,t,g);let b=e.push("blockquote_close","blockquote",-1);b.markup=">",e.lineMax=o,e.parentType=f,p[1]=e.line;for(let y=0;y<c.length;y++)e.bMarks[y+t]=s[y],e.tShift[y+t]=c[y],e.sCount[y+t]=u[y],e.bsCount[y+t]=l[y];return e.blkIndent=m,!0}function hm(e,t,n,r){let a=e.eMarks[t];if(e.sCount[t]-e.blkIndent>=4)return!1;let i=e.bMarks[t]+e.tShift[t],o=e.src.charCodeAt(i++);if(o!==42&&o!==45&&o!==95)return!1;let s=1;for(;i<a;){let u=e.src.charCodeAt(i++);if(u!==o&&!ne(u))return!1;u===o&&s++}if(s<3)return!1;if(r)return!0;e.line=t+1;let l=e.push("hr","hr",0);return l.map=[t,e.line],l.markup=Array(s+1).join(String.fromCharCode(o)),!0}function Bu(e,t){let n=e.eMarks[t],r=e.bMarks[t]+e.tShift[t],a=e.src.charCodeAt(r++);return a!==42&&a!==45&&a!==43||r<n&&!ne(e.src.charCodeAt(r))?-1:r}function Wu(e,t){let n=e.bMarks[t]+e.tShift[t],r=e.eMarks[t],a=n;if(a+1>=r)return-1;let i=e.src.charCodeAt(a++);if(i<48||i>57)return-1;for(;;){if(a>=r)return-1;if(i=e.src.charCodeAt(a++),i>=48&&i<=57){if(a-n>=10)return-1;continue}if(i===41||i===46)break;return-1}return a<r&&(i=e.src.charCodeAt(a),!ne(i))?-1:a}function fm(e,t){let n=e.level+2;for(let r=t+2,a=e.tokens.length-2;r<a;r++)e.tokens[r].level===n&&e.tokens[r].type==="paragraph_open"&&(e.tokens[r+2].hidden=!0,e.tokens[r].hidden=!0,r+=2)}function gm(e,t,n,r){let a,i,o,s,l=t,u=!0;if(e.sCount[l]-e.blkIndent>=4||e.listIndent>=0&&e.sCount[l]-e.listIndent>=4&&e.sCount[l]<e.blkIndent)return!1;let c=!1;r&&e.parentType==="paragraph"&&e.sCount[l]>=e.blkIndent&&(c=!0);let d,f,h;if((h=Wu(e,l))>=0){if(d=!0,o=e.bMarks[l]+e.tShift[l],f=Number(e.src.slice(o,h-1)),c&&f!==1)return!1}else if((h=Bu(e,l))>=0)d=!1;else return!1;if(c&&e.skipSpaces(h)>=e.eMarks[l])return!1;if(r)return!0;let g=e.src.charCodeAt(h-1),m=e.tokens.length;d?(s=e.push("ordered_list_open","ol",1),f!==1&&(s.attrs=[["start",f]])):s=e.push("bullet_list_open","ul",1);let v=[l,0];s.map=v,s.markup=String.fromCharCode(g);let p=!1,b=e.md.block.ruler.getRules("list"),y=e.parentType;for(e.parentType="list";l<n;){i=h,a=e.eMarks[l];let _=e.sCount[l]+h-(e.bMarks[l]+e.tShift[l]),x=_;for(;i<a;){let L=e.src.charCodeAt(i);if(L===9)x+=4-(x+e.bsCount[l])%4;else if(L===32)x++;else break;i++}let T=i,E;T>=a?E=1:E=x-_,E>4&&(E=1);let S=_+E;s=e.push("list_item_open","li",1),s.markup=String.fromCharCode(g);let w=[l,0];s.map=w,d&&(s.info=e.src.slice(o,h-1));let A=e.tight,C=e.tShift[l],D=e.sCount[l],F=e.listIndent;if(e.listIndent=e.blkIndent,e.blkIndent=S,e.tight=!0,e.tShift[l]=T-e.bMarks[l],e.sCount[l]=x,T>=a&&e.isEmpty(l+1)?e.line=Math.min(e.line+2,n):e.md.block.tokenize(e,l,n),(!e.tight||p)&&(u=!1),p=e.line-l>1&&e.isEmpty(e.line-1),e.blkIndent=e.listIndent,e.listIndent=F,e.tShift[l]=C,e.sCount[l]=D,e.tight=A,s=e.push("list_item_close","li",-1),s.markup=String.fromCharCode(g),l=e.line,w[1]=l,l>=n||e.sCount[l]<e.blkIndent||e.sCount[l]-e.blkIndent>=4)break;let k=!1;for(let L=0,z=b.length;L<z;L++)if(b[L](e,l,n,!0)){k=!0;break}if(k)break;if(d){if(h=Wu(e,l),h<0)break;o=e.bMarks[l]+e.tShift[l]}else if(h=Bu(e,l),h<0)break;if(g!==e.src.charCodeAt(h-1))break}return d?s=e.push("ordered_list_close","ol",-1):s=e.push("bullet_list_close","ul",-1),s.markup=String.fromCharCode(g),v[1]=l,e.line=l,e.parentType=y,u&&fm(e,m),!0}var wi=999;function pm(e){let t=e.length;for(let n=0;n<e.length-1;n++){let r=e.charCodeAt(n);if(r<55296||r>56319)continue;let a=e.charCodeAt(n+1);a>=56320&&a<=57343&&(t--,n++)}return t}function In(e,t){return Math.min(e.eMarks[t]+1,e.src.length)}function ea(e,t){let n=e.lineMax;if(t>=n||e.isEmpty(t))return-1;let r=!1;if(e.sCount[t]-e.blkIndent>3&&(r=!0),e.sCount[t]<0&&(r=!0),!r){let a=e.md.block.ruler.getRules("reference"),i=e.parentType;e.parentType="reference";let o=!1;for(let s=0,l=a.length;s<l;s++)if(a[s](e,t,n,!0)){o=!0;break}if(e.parentType=i,o)return-1}return e.bMarks[t]+e.tShift[t]}function vm(e,t,n,r){let a=e.src,i=e.bMarks[t]+e.tShift[t],o=t+1;if(e.sCount[t]-e.blkIndent>=4||a.charCodeAt(i)!==91)return!1;let s=In(e,t),l="",u=-1,c=++i;for(;i<s;){if(l.length+i-c>wi*2)return!1;let T=a.charCodeAt(i);if(T===91)return!1;if(T===93){u=i;break}if(T===92&&(T=a.charCodeAt(++i)),T===10&&i<s){let E=ea(e,o);if(E<0)return!1;l+=a.slice(c,s),i=c=E,s=In(e,o++);continue}i++}if(u<0||a.charCodeAt(u+1)!==58||(l+=a.slice(c,u),l.length>wi&&pm(l)>wi))return!1;for(i=u+2;i<s;){let T=a.charCodeAt(i);if(T===10){let E=ea(e,o);if(E<0)break;i=E,s=In(e,o++)}else if(ne(T))i++;else break}let d=e.md.helpers.parseLinkDestination(a,i,s);if(!d.ok)return!1;let f=e.md.normalizeLink(d.str);if(!e.md.validateLink(f))return!1;i=d.pos;let h=i,g=s,m=o;for(;i<s;){let T=a.charCodeAt(i);if(T===10){let E=ea(e,o);if(E<0)break;i=E,s=In(e,o++)}else if(ne(T))i++;else break}let v=i,p=e.md.helpers.parseLinkTitle(a,i,s);for(;p.can_continue;){let T=ea(e,o);if(T<0)break;i=T,s=In(e,o++),p=e.md.helpers.parseLinkTitle(a,i,s,p)}let b;for(v!==h&&p.ok?(b=p.str,i=p.pos):(b="",i=h,s=g,o=m);i<s&&ne(a.charCodeAt(i));)i++;if(i<s&&a.charCodeAt(i)!==10&&b)for(b="",i=h,s=g,o=m;i<s&&ne(a.charCodeAt(i));)i++;if(i<s&&a.charCodeAt(i)!==10)return!1;let y=na(l);if(!y)return!1;if(r)return!0;typeof e.env.references>"u"&&(e.env.references={}),typeof e.env.references[y]>"u"&&(e.env.references[y]={title:b,href:f});let _=e.push("reference_definition","",0);_.map=[t,o],_.hidden=!0;let x=Object.create(null);return x.label=y,_.meta=x,e.line=o,!0}var mm=["address","article","aside","base","basefont","blockquote","body","caption","center","col","colgroup","dd","details","dialog","dir","div","dl","dt","fieldset","figcaption","figure","footer","form","frame","frameset","h1","h2","h3","h4","h5","h6","head","header","hr","html","iframe","legend","li","link","main","menu","menuitem","nav","noframes","ol","optgroup","option","p","param","search","section","summary","table","tbody","td","tfoot","th","thead","title","tr","track","ul"],ec=`<[A-Za-z][A-Za-z0-9\\-]*(?:\\s+[a-zA-Z_:][a-zA-Z0-9:._-]*(?:\\s*=\\s*(?:[^"'=<>\`\\x00-\\x20]+|'[^']*'|"[^"]*"))?)*\\s*\\/?>`,tc="<\\/[A-Za-z][A-Za-z0-9\\-]*\\s*>";new RegExp(`^(?:${ec}|${tc}|<!---?>|<!--(?:[^-]|-[^-]|--[^>])*-->|<[?][\\s\\S]*?[?]>|<![A-Za-z][^>]*>|<!\\[CDATA\\[[\\s\\S]*?\\]\\]>)`);var nc=new RegExp(`^(?:${ec}|${tc})`),Rt=[[/^<(script|pre|style|textarea)(?=(\s|>|$))/i,/<\/(script|pre|style|textarea)>/i,!0],[/^<!--/,/-->/,!0],[/^<\?/,/\?>/,!0],[/^<![A-Za-z]/,/>/,!0],[/^<!\[CDATA\[/,/\]\]>/,!0],[new RegExp(`^</?(${mm.join("|")})(?=(\\s|/?>|$))`,"i"),/^$/,!0],[new RegExp(`${nc.source}\\s*$`),/^$/,!1]];function bm(e,t,n,r){let a=e.bMarks[t]+e.tShift[t],i=e.eMarks[t];if(e.sCount[t]-e.blkIndent>=4||!e.md.options.html||e.src.charCodeAt(a)!==60)return!1;let o=e.src.slice(a,i),s=0;for(;s<Rt.length&&!Rt[s][0].test(o);s++);if(s===Rt.length)return!1;if(r)return Rt[s][2];let l=t+1,u=Rt[s][1].test("");if(!Rt[s][1].test(o)){for(;l<n&&!(e.sCount[l]<e.blkIndent&&(u||!e.isEmpty(l)));l++)if(a=e.bMarks[l]+e.tShift[l],i=e.eMarks[l],o=e.src.slice(a,i),Rt[s][1].test(o)){o.length!==0&&l++;break}}e.line=l;let c=e.push("html_block","",0);return c.map=[t,l],c.content=e.getLines(t,l,e.blkIndent,!0),!0}function ym(e,t,n,r){let a=e.bMarks[t]+e.tShift[t],i=e.eMarks[t];if(e.sCount[t]-e.blkIndent>=4)return!1;let o=e.src.charCodeAt(a);if(o!==35||a>=i)return!1;let s=1;for(o=e.src.charCodeAt(++a);o===35&&a<i&&s<=6;)s++,o=e.src.charCodeAt(++a);if(s>6||a<i&&!ne(o))return!1;if(r)return!0;i=e.skipSpacesBack(i,a);let l=e.skipCharsBack(i,35,a);l>a&&ne(e.src.charCodeAt(l-1))&&(i=l),e.line=t+1;let u=e.push("heading_open",`h${s}`,1);u.markup="########".slice(0,s),u.map=[t,e.line];let c=e.push("inline","",0);c.content=ra(e.src.slice(a,i)),c.map=[t,e.line],c.children=[];let d=e.push("heading_close",`h${s}`,-1);return d.markup="########".slice(0,s),!0}function _m(e,t,n){let r=e.md.block.ruler.getRules("paragraph");if(e.sCount[t]-e.blkIndent>=4)return!1;let a=e.parentType;e.parentType="paragraph";let i=0,o,s=t+1;for(;s<n&&!e.isEmpty(s);s++){if(e.sCount[s]-e.blkIndent>3)continue;if(e.sCount[s]>=e.blkIndent){let h=e.bMarks[s]+e.tShift[s],g=e.eMarks[s];if(h<g&&(o=e.src.charCodeAt(h),(o===45||o===61)&&(h=e.skipChars(h,o),h=e.skipSpaces(h),h>=g))){i=o===61?1:2;break}}if(e.sCount[s]<0)continue;let f=!1;for(let h=0,g=r.length;h<g;h++)if(r[h](e,s,n,!0)){f=!0;break}if(f)break}if(!i)return e.parentType=a,!1;let l=ra(e.getLines(t,s,e.blkIndent,!1));e.line=s+1;let u=e.push("heading_open",`h${i}`,1);u.markup=String.fromCharCode(o),u.map=[t,e.line];let c=e.push("inline","",0);c.content=l,c.map=[t,e.line-1],c.children=[];let d=e.push("heading_close",`h${i}`,-1);return d.markup=String.fromCharCode(o),e.parentType=a,!0}function xm(e,t,n){let r=e.md.block.ruler.getRules("paragraph"),a=e.parentType,i=t+1;for(e.parentType="paragraph";i<n&&!e.isEmpty(i);i++){if(e.sCount[i]-e.blkIndent>3||e.sCount[i]<0)continue;let u=!1;for(let c=0,d=r.length;c<d;c++)if(r[c](e,i,n,!0)){u=!0;break}if(u)break}let o=ra(e.getLines(t,i,e.blkIndent,!1));e.line=i;let s=e.push("paragraph_open","p",1);s.map=[t,e.line];let l=e.push("inline","",0);return l.content=o,l.map=[t,e.line],l.children=[],e.push("paragraph_close","p",-1),e.parentType=a,!0}var ta=[["table",lm,["paragraph","reference"]],["code",um],["fence",cm,["paragraph","reference","blockquote","list"]],["blockquote",dm,["paragraph","reference","blockquote","list"]],["hr",hm,["paragraph","reference","blockquote","list"]],["list",gm,["paragraph","reference","blockquote"]],["reference",vm],["html_block",bm,["paragraph","reference","blockquote"]],["heading",ym,["paragraph","reference","blockquote"]],["lheading",_m],["paragraph",xm]],rc=class{constructor(){H(this,"ruler",new Wn),H(this,"State",Qu);for(let e=0;e<ta.length;e++)this.ruler.push(ta[e][0],ta[e][1],{alt:(ta[e][2]||[]).slice()})}tokenize(e,t,n){let r=this.ruler.getRules(""),a=r.length,i=e.md.options.maxNesting,o=t,s=!1;for(;o<n&&(e.line=o=e.skipEmptyLines(o),!(o>=n||e.sCount[o]<e.blkIndent));){if(e.level>=i){e.line=n;break}let l=e.line,u=!1;for(let c=0;c<a;c++)if(u=r[c](e,o,n,!1),u){if(l>=e.line)throw new Error("block rule didn't increment state.line");break}if(!u)throw new Error("none of the block rules matched");e.tight=!s,e.isEmpty(e.line-1)&&(s=!0),o=e.line,o<n&&e.isEmpty(o)&&(s=!0,o++,e.line=o)}}parse(e,t,n,r){if(!e)return;let a=new this.State(e,t,n,r);this.tokenize(a,a.line,a.lineMax)}},ac=class{constructor(e,t,n,r){H(this,"pos",0),H(this,"level",0),H(this,"pending",""),H(this,"pendingLevel",0),H(this,"cache",{}),H(this,"backticks",{}),H(this,"backticksScanned",!1),H(this,"skipHtmlSpecial",!1),H(this,"linkLevel",0),H(this,"delimiters",[]),H(this,"_prev_delimiters",[]),H(this,"Token",At),this.src=e,this.env=n,this.md=t,this.tokens=r,this.tokens_meta=Array(r.length),this.posMax=this.src.length}pushPending(){let e=new At("text","",0);return e.content=this.pending,e.level=this.pendingLevel,this.tokens.push(e),this.pending="",e}push(e,t,n){this.pending&&this.pushPending();let r=new At(e,t,n),a;return n<0&&(this.level--,this.delimiters=this._prev_delimiters.pop()),r.level=this.level,n>0&&(this.level++,this._prev_delimiters.push(this.delimiters),this.delimiters=[],a={delimiters:this.delimiters}),this.pendingLevel=this.level,this.tokens.push(r),this.tokens_meta.push(a),r}scanDelims(e,t){let n=this.posMax,r=this.src.charCodeAt(e),a;if(e===0)a=32;else if(e===1)a=this.src.charCodeAt(0),(a&63488)===55296&&(a=65533);else if(a=this.src.charCodeAt(e-1),(a&64512)===56320){let g=this.src.charCodeAt(e-2);a=(g&64512)===55296?65536+(g-55296<<10)+(a-56320):65533}else(a&64512)===55296&&(a=65533);let i=e;for(;i<n&&this.src.charCodeAt(i)===r;)i++;let o=i-e,s=i<n?this.src.charCodeAt(i):32;if((s&64512)===55296){let g=this.src.charCodeAt(i+1);s=(g&64512)===56320?65536+(s-55296<<10)+(g-56320):65533}else(s&64512)===56320&&(s=65533);let l=Gn(a)||Mn(a),u=Gn(s)||Mn(s),c=zn(a),d=zn(s),f=!d&&(!u||c||l),h=!c&&(!l||d||u);return{can_open:f&&(t||!h||l),can_close:h&&(t||!f||u),length:o}}};function Tm(e){switch(e){case 10:case 33:case 35:case 36:case 37:case 38:case 42:case 43:case 45:case 58:case 60:case 61:case 62:case 64:case 91:case 92:case 93:case 94:case 95:case 96:case 123:case 125:case 126:return!0;default:return!1}}function Em(e,t){let n=e.pos;for(;n<e.posMax&&!Tm(e.src.charCodeAt(n));)n++;return n===e.pos?!1:(t||(e.pending+=e.src.slice(e.pos,n)),e.pos=n,!0)}function Sm(e){return e>=65&&e<=90||e>=97&&e<=122}function Rm(e){return e>=65&&e<=90||e>=97&&e<=122||e>=48&&e<=57||e===43||e===45||e===46}function Am(e,t){if(!e.md.options.linkify||e.linkLevel>0)return!1;let n=e.pos,r=e.posMax;if(n+3>r||e.src.charCodeAt(n)!==58||e.src.charCodeAt(n+1)!==47||e.src.charCodeAt(n+2)!==47)return!1;let a=n-Math.min(10,e.pending.length,n),i=n;for(;i>a&&Rm(e.src.charCodeAt(i-1));)i--;if(i===n||!Sm(e.src.charCodeAt(i)))return!1;let o=n-i,s=e.md.linkify.matchAtStart(e.src.slice(i));if(!s)return!1;let l=s.url;if(l.length<=o)return!1;let u=l.length;for(;u>0&&l.charCodeAt(u-1)===42;)u--;u!==l.length&&(l=l.slice(0,u));let c=e.md.normalizeLink(l);if(!e.md.validateLink(c))return!1;if(!t){e.pending=e.pending.slice(0,-o);let d=e.push("link_open","a",1);d.attrs=[["href",c]],d.markup="linkify",d.info="auto";let f=e.push("text","",0);f.content=e.md.normalizeLinkText(l);let h=e.push("link_close","a",-1);h.markup="linkify",h.info="auto"}return e.pos+=l.length-o,!0}function wm(e,t){let n=e.pos;if(e.src.charCodeAt(n)!==10)return!1;let r=e.pending.length-1,a=e.posMax;if(!t)if(r>=0&&e.pending.charCodeAt(r)===32)if(r>=1&&e.pending.charCodeAt(r-1)===32){let i=r-1;for(;i>=1&&e.pending.charCodeAt(i-1)===32;)i--;e.pending=e.pending.slice(0,i),e.push("hardbreak","br",0)}else e.pending=e.pending.slice(0,-1),e.push("softbreak","br",0);else e.push("softbreak","br",0);for(n++;n<a&&ne(e.src.charCodeAt(n));)n++;return e.pos=n,!0}var Li=[];for(let e=0;e<256;e++)Li.push(0);"\\!\"#$%&'()*+,./:;<=>?@[]^_`{|}~-".split("").forEach(function(e){Li[e.charCodeAt(0)]=1});function Dm(e,t){let n=e.pos,r=e.posMax;if(e.src.charCodeAt(n)!==92||(n++,n>=r))return!1;let a=e.src.charCodeAt(n);if(a===10){for(t||e.push("hardbreak","br",0),n++;n<r&&(a=e.src.charCodeAt(n),!!ne(a));)n++;return e.pos=n,!0}if(a===32){if(!t){let s=e.push("text_special","",0);s.content="\\",s.markup="\\",s.info="escape"}return e.pos=n,!0}let i=e.src[n];if(a>=55296&&a<=56319&&n+1<r){let s=e.src.charCodeAt(n+1);s>=56320&&s<=57343&&(i+=e.src[n+1],n++)}let o="\\"+i;if(!t){let s=e.push("text_special","",0);a<256&&Li[a]!==0?s.content=i:s.content=o,s.markup=o,s.info="escape"}return e.pos=n+1,!0}function Cm(e){let t={},n=0;for(;(n=e.indexOf("`",n))!==-1;){let r=n;for(;e.charCodeAt(++n)===96;);t[n-r]=r}return t}function km(e,t){var n;let r=e.pos;if(e.src.charCodeAt(r)!==96)return!1;let a=e.posMax,i=r+1;for(;i<a&&e.src.charCodeAt(i)===96;)i++;let o=e.src.slice(r,i),s=o.length;if(e.backticksScanned||(e.backticks=Cm(e.src),e.backticksScanned=!0),((n=e.backticks[s])!==null&&n!==void 0?n:-1)>=i){let l=i,u;for(;(u=e.src.indexOf("`",l))!==-1&&u<a;){for(l=u+1;e.src.charCodeAt(l)===96;)l++;if(l>a)break;if(l-u===s){if(!t){let c=e.push("code_inline","code",0);c.markup=o;let d=e.src.slice(i,u).replace(/\n/g," ");d.startsWith(" ")&&d.endsWith(" ")&&/[^ ]/.test(d)&&(d=d.slice(1,-1)),c.content=d}return e.pos=l,!0}}}return t||(e.pending+=o),e.pos=i,!0}function Lm(e,t){let n=e.pos,r=e.src.charCodeAt(n);if(t||r!==126)return!1;let a=e.scanDelims(e.pos,!0),i=a.length,o=String.fromCharCode(r);if(i<2)return!1;let s;i%2&&(s=e.push("text","",0),s.content=o,i--);for(let l=0;l<i;l+=2)s=e.push("text","",0),s.content=o+o,e.delimiters.push({marker:r,length:0,token:e.tokens.length-1,end:-1,open:a.can_open,close:a.can_close});return e.pos+=a.length,!0}function Ou(e,t){let n,r=[],a=t.length;for(let i=0;i<a;i++){let o=t[i];if(o.marker!==126||o.end===-1)continue;let s=t[o.end];n=e.tokens[o.token],n.type="s_open",n.tag="s",n.nesting=1,n.markup="~~",n.content="",n=e.tokens[s.token],n.type="s_close",n.tag="s",n.nesting=-1,n.markup="~~",n.content="",e.tokens[s.token-1].type==="text"&&e.tokens[s.token-1].content==="~"&&r.push(s.token-1)}for(;r.length;){let i=r.pop(),o=i+1;for(;o<e.tokens.length&&e.tokens[o].type==="s_close";)o++;o--,i!==o&&(n=e.tokens[o],e.tokens[o]=e.tokens[i],e.tokens[i]=n)}}function Fm(e){let t=e.tokens_meta,n=e.tokens_meta.length;Ou(e,e.delimiters);for(let a=0;a<n;a++){var r;let i=(r=t[a])===null||r===void 0?void 0:r.delimiters;i&&Ou(e,i)}}var ic={tokenize:Lm,postProcess:Fm};function Pm(e,t){let n=e.pos,r=e.src.charCodeAt(n);if(t||r!==95&&r!==42)return!1;let a=e.scanDelims(e.pos,r===42);for(let i=0;i<a.length;i++){let o=e.push("text","",0);o.content=String.fromCharCode(r),e.delimiters.push({marker:r,length:a.length,token:e.tokens.length-1,end:-1,open:a.can_open,close:a.can_close})}return e.pos+=a.length,!0}function Uu(e,t){let n=t.length;for(let r=n-1;r>=0;r--){let a=t[r];if(a.marker!==95&&a.marker!==42||a.end===-1)continue;let i=t[a.end],o=r>0&&t[r-1].end===a.end+1&&t[r-1].marker===a.marker&&t[r-1].token===a.token-1&&t[a.end+1].token===i.token+1,s=String.fromCharCode(a.marker),l=e.tokens[a.token];l.type=o?"strong_open":"em_open",l.tag=o?"strong":"em",l.nesting=1,l.markup=o?s+s:s,l.content="";let u=e.tokens[i.token];u.type=o?"strong_close":"em_close",u.tag=o?"strong":"em",u.nesting=-1,u.markup=o?s+s:s,u.content="",o&&(e.tokens[t[r-1].token].content="",e.tokens[t[a.end+1].token].content="",r--)}}function Im(e){let t=e.tokens_meta,n=e.tokens_meta.length;Uu(e,e.delimiters);for(let a=0;a<n;a++){var r;let i=(r=t[a])===null||r===void 0?void 0:r.delimiters;i&&Uu(e,i)}}var oc={tokenize:Pm,postProcess:Im};function Nm(e,t){let n,r,a,i,o="",s="",l=e.pos,u=!0;if(e.src.charCodeAt(e.pos)!==91)return!1;let c=e.pos,d=e.posMax,f=e.pos+1,h=e.md.helpers.parseLinkLabel(e,e.pos,!0);if(h<0)return!1;let g=h+1;if(g<d&&e.src.charCodeAt(g)===40){for(u=!1,g++;g<d&&(n=e.src.charCodeAt(g),!(!ne(n)&&n!==10));g++);if(g>=d)return!1;if(l=g,a=e.md.helpers.parseLinkDestination(e.src,g,e.posMax),a.ok){for(o=e.md.normalizeLink(a.str),e.md.validateLink(o)?g=a.pos:o="",l=g;g<d&&(n=e.src.charCodeAt(g),!(!ne(n)&&n!==10));g++);if(a=e.md.helpers.parseLinkTitle(e.src,g,e.posMax),g<d&&l!==g&&a.ok)for(s=a.str,g=a.pos;g<d&&(n=e.src.charCodeAt(g),!(!ne(n)&&n!==10));g++);}(g>=d||e.src.charCodeAt(g)!==41)&&(u=!0),g++}if(u){if(typeof e.env.references>"u")return!1;if(g<d&&e.src.charCodeAt(g)===91?(l=g+1,g=e.md.helpers.parseLinkLabel(e,g),g>=0?r=e.src.slice(l,g++):g=h+1):g=h+1,r||(r=e.src.slice(f,h)),r=na(r),i=e.env.references[r],!i)return e.pos=c,!1;o=i.href,s=i.title}if(!t){e.pos=f,e.posMax=h;let m=e.push("link_open","a",1),v=[["href",o]];if(m.attrs=v,s&&v.push(["title",s]),r){let p=Object.create(null);p.label=r,m.meta=p}e.linkLevel++,e.md.inline.tokenize(e),e.linkLevel--,e.push("link_close","a",-1)}return e.pos=g,e.posMax=d,!0}function zm(e,t){let n,r,a,i,o,s,l,u,c="",d=e.pos,f=e.posMax;if(e.src.charCodeAt(e.pos)!==33||e.src.charCodeAt(e.pos+1)!==91)return!1;let h=e.pos+2,g=e.md.helpers.parseLinkLabel(e,e.pos+1,!1);if(g<0)return!1;if(i=g+1,i<f&&e.src.charCodeAt(i)===40){for(i++;i<f&&(n=e.src.charCodeAt(i),!(!ne(n)&&n!==10));i++);if(i>=f)return!1;for(u=i,s=e.md.helpers.parseLinkDestination(e.src,i,e.posMax),s.ok&&(c=e.md.normalizeLink(s.str),e.md.validateLink(c)?i=s.pos:c=""),u=i;i<f&&(n=e.src.charCodeAt(i),!(!ne(n)&&n!==10));i++);if(s=e.md.helpers.parseLinkTitle(e.src,i,e.posMax),i<f&&u!==i&&s.ok)for(l=s.str,i=s.pos;i<f&&(n=e.src.charCodeAt(i),!(!ne(n)&&n!==10));i++);else l="";if(i>=f||e.src.charCodeAt(i)!==41)return e.pos=d,!1;i++}else{if(typeof e.env.references>"u")return!1;if(i<f&&e.src.charCodeAt(i)===91?(u=i+1,i=e.md.helpers.parseLinkLabel(e,i),i>=0?a=e.src.slice(u,i++):i=g+1):i=g+1,a||(a=e.src.slice(h,g)),a=na(a),o=e.env.references[a],!o)return e.pos=d,!1;c=o.href,l=o.title}if(!t){r=e.src.slice(h,g);let m=[];e.md.inline.parse(r,e.md,e.env,m);let v=e.push("image","img",0),p=[["src",c],["alt",""]];if(v.attrs=p,v.children=m,v.content=r,l&&p.push(["title",l]),a){let b=Object.create(null);b.label=a,v.meta=b}}return e.pos=i,e.posMax=f,!0}var Mm=/^([a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*)$/,Gm=/^([a-zA-Z][a-zA-Z0-9+.-]{1,31}):([^<>\x00-\x20]*)$/;function Bm(e,t){let n=e.pos;if(e.src.charCodeAt(n)!==60)return!1;let r=e.pos,a=e.posMax;for(;;){if(++n>=a)return!1;let o=e.src.charCodeAt(n);if(o===60)return!1;if(o===62)break}let i=e.src.slice(r+1,n);if(Gm.test(i)){let o=e.md.normalizeLink(i);if(!e.md.validateLink(o))return!1;if(!t){let s=e.push("link_open","a",1);s.attrs=[["href",o]],s.markup="autolink",s.info="auto";let l=e.push("text","",0);l.content=e.md.normalizeLinkText(i);let u=e.push("link_close","a",-1);u.markup="autolink",u.info="auto"}return e.pos+=i.length+2,!0}if(Mm.test(i)){let o=e.md.normalizeLink(`mailto:${i}`);if(!e.md.validateLink(o))return!1;if(!t){let s=e.push("link_open","a",1);s.attrs=[["href",o]],s.markup="autolink",s.info="auto";let l=e.push("text","",0);l.content=e.md.normalizeLinkText(i);let u=e.push("link_close","a",-1);u.markup="autolink",u.info="auto"}return e.pos+=i.length+2,!0}return!1}function Wm(e){return/^<a[>\s]/i.test(e)}function Om(e){return/^<\/a\s*>/i.test(e)}function sc(e){let t=e|32;return t>=97&&t<=122}function Um(e,t){let n=e.src,r,a;if(n.startsWith("<!--",t)){if(n.startsWith(">",t+4))return t+5;if(n.startsWith("->",t+4))return t+6;r=t+4,a="-->"}else if(n.startsWith("<![CDATA[",t))r=t+9,a="]]>";else if(n.charCodeAt(t+1)===63)r=t+2,a="?>";else if(sc(n.charCodeAt(t+2)))r=t+3,a=">";else return-1;if(e.skipHtmlSpecial)return-1;let i=n.indexOf(a,r);return i<0?(e.skipHtmlSpecial=!0,-1):i+a.length}function Hm(e,t){if(!e.md.options.html)return!1;let n=e.posMax,r=e.pos;if(e.src.charCodeAt(r)!==60||r+2>=n)return!1;let a,i=e.src.charCodeAt(r+1);if(i===33||i===63)a=Um(e,r);else if(i===47||sc(i)){let o=e.src.slice(r).match(nc);a=o?r+o[0].length:-1}else return!1;if(a<0)return!1;if(!t){let o=e.push("html_inline","",0);o.content=e.src.slice(r,a),Wm(o.content)&&e.linkLevel++,Om(o.content)&&e.linkLevel--}return e.pos=a,!0}var jm=/^&#((?:x[a-f0-9]{1,6}|[0-9]{1,7}));/i,$m=/^&([a-z][a-z0-9]{1,31});/i;function Vm(e,t){let n=e.pos,r=e.posMax;if(e.src.charCodeAt(n)!==38||n+1>=r)return!1;if(e.src.charCodeAt(n+1)===35){let a=e.src.slice(n).match(jm);if(a){if(!t){let i=a[1][0].toLowerCase()==="x"?parseInt(a[1].slice(1),16):parseInt(a[1],10),o=e.push("text_special","",0);o.content=ki(i)?Nn(i):Nn(65533),o.markup=a[0],o.info="entity"}return e.pos+=a[0].length,!0}}else{let a=e.src.slice(n).match($m);if(a){let i=Jr(a[0]);if(i!==a[0]){if(!t){let o=e.push("text_special","",0);o.content=i,o.markup=a[0],o.info="entity"}return e.pos+=a[0].length,!0}}}return!1}function Hu(e){let t={},n=e.length;if(!n)return;let r=0,a=-2,i=[];for(let o=0;o<n;o++){let s=e[o];if(i.push(0),(e[r].marker!==s.marker||a!==s.token-1)&&(r=o),a=s.token,s.length=s.length||0,!s.close)continue;t.hasOwnProperty(s.marker)||(t[s.marker]=[-1,-1,-1,-1,-1,-1]);let l=t[s.marker][(s.open?3:0)+s.length%3],u=r-i[r]-1,c=u;for(;u>l;u-=i[u]+1){let d=e[u];if(d.marker===s.marker&&d.open&&d.end<0){let f=!1;if((d.close||s.open)&&(d.length+s.length)%3===0&&(d.length%3!==0||s.length%3!==0)&&(f=!0),!f){let h=u>0&&!e[u-1].open?i[u-1]+1:0;i[o]=o-u+h,i[u]=h,s.open=!1,d.end=o,d.close=!1,c=-1,a=-2;break}}}c!==-1&&(t[s.marker][(s.open?3:0)+(s.length||0)%3]=c)}}function qm(e){let t=e.tokens_meta,n=e.tokens_meta.length;Hu(e.delimiters);for(let a=0;a<n;a++){var r;let i=(r=t[a])===null||r===void 0?void 0:r.delimiters;i&&Hu(i)}}function Xm(e){let t,n,r=0,a=e.tokens,i=e.tokens.length;for(t=n=0;t<i;t++)a[t].nesting<0&&r--,a[t].level=r,a[t].nesting>0&&r++,a[t].type==="text"&&t+1<i&&a[t+1].type==="text"?a[t+1].content=a[t].content+a[t+1].content:(t!==n&&(a[n]=a[t]),n++);t!==n&&(a.length=n)}var Di=[["text",Em],["linkify",Am],["newline",wm],["escape",Dm],["backticks",km],["strikethrough",ic.tokenize],["emphasis",oc.tokenize],["link",Nm],["image",zm],["autolink",Bm],["html_inline",Hm],["entity",Vm]],Ci=[["balance_pairs",qm],["strikethrough",ic.postProcess],["emphasis",oc.postProcess],["fragments_join",Xm]],lc=class{constructor(){H(this,"ruler",new Wn),H(this,"ruler2",new Wn),H(this,"State",ac);for(let e=0;e<Di.length;e++)this.ruler.push(Di[e][0],Di[e][1]);for(let e=0;e<Ci.length;e++)this.ruler2.push(Ci[e][0],Ci[e][1])}skipToken(e){let t=e.pos,n=this.ruler.getRules(""),r=n.length,a=e.md.options.maxNesting,i=e.cache;if(typeof i[t]<"u"){e.pos=i[t];return}let o=!1;if(e.level<a){for(let s=0;s<r;s++)if(e.level++,o=n[s](e,!0),e.level--,o){if(t>=e.pos)throw new Error("inline rule didn't increment state.pos");break}}else e.pos=e.posMax;o||e.pos++,i[t]=e.pos}tokenize(e){let t=this.ruler.getRules(""),n=t.length,r=e.posMax,a=e.md.options.maxNesting;for(;e.pos<r;){let i=e.pos,o=!1;if(e.level<a){for(let s=0;s<n;s++)if(o=t[s](e,!1),o){if(i>=e.pos)throw new Error("inline rule didn't increment state.pos");break}}if(o){if(e.pos>=r)break;continue}e.pending+=e.src[e.pos++]}e.pending&&e.pushPending()}parse(e,t,n,r){let a=new this.State(e,t,n,r);this.tokenize(a);let i=this.ruler2.getRules(""),o=i.length;for(let s=0;s<o;s++)i[s](a)}},Ym={default:{options:{html:!1,xhtmlOut:!1,breaks:!1,langPrefix:"language-",linkify:!1,typographer:!1,quotes:"\u201C\u201D\u2018\u2019",highlight:null,maxNesting:100},components:{core:{},block:{},inline:{}}},zero:{options:{html:!1,xhtmlOut:!1,breaks:!1,langPrefix:"language-",linkify:!1,typographer:!1,quotes:"\u201C\u201D\u2018\u2019",highlight:null,maxNesting:20},components:{core:{rules:["normalize","block","strip_references","inline","text_join"]},block:{rules:["paragraph"]},inline:{rules:["text"],rules2:["balance_pairs","fragments_join"]}}},commonmark:{options:{html:!0,xhtmlOut:!0,breaks:!1,langPrefix:"language-",linkify:!1,typographer:!1,quotes:"\u201C\u201D\u2018\u2019",highlight:null,maxNesting:20},components:{core:{rules:["normalize","block","strip_references","inline","text_join"]},block:{rules:["blockquote","code","fence","heading","hr","html_block","lheading","list","reference","paragraph"]},inline:{rules:["autolink","backticks","emphasis","entity","escape","html_inline","image","link","newline","text"],rules2:["balance_pairs","emphasis","fragments_join"]}}}},Km=/^(vbscript|javascript|file|data):/,Zm=/^data:image\/(gif|png|jpeg|webp);/,ju=["http:","https:","mailto:"],$e=class{validateLink(e){let t=e.trim().toLowerCase();return Km.test(t)?Zm.test(t):!0}normalizeLink(e){let t=Fn(e,!0);if(t.hostname&&(!t.protocol||ju.indexOf(t.protocol)>=0))try{t.hostname=Si.toASCII(t.hostname)}catch{}return t.auth&&(t.auth=st(t.auth)),t.hostname&&(t.hostname=st(t.hostname)),t.pathname&&(t.pathname=st(t.pathname)),t.search&&(t.search=st(t.search)),t.hash&&(t.hash=st(t.hash)),Zt(t)}normalizeLinkText(e){let t=Fn(e,!0);if(t.hostname&&(!t.protocol||ju.indexOf(t.protocol)>=0))try{t.hostname=Si.toUnicode(t.hostname)}catch{}return Ln(Zt(t),Ln.defaultChars+"%")}constructor(...e){H(this,"inline",new lc),H(this,"block",new rc),H(this,"core",new Ju),H(this,"renderer",new Yu),H(this,"linkify",new Au),H(this,"utils",yv),H(this,"helpers",Object.assign({},Nv));let[t,n]=e;typeof t=="string"?(this.configure(t),n&&this.set(n)):(this.configure("default"),this.set(t||{}))}set(e){return Object.assign(this.options,e),this}configure(e){let t;if(typeof e=="string"){let a=e;if(t=Ym[a],!t)throw new Error(`Wrong 'markdown-it' preset "${a}", check name`)}else t=e;if(!t)throw new Error("Wrong `markdown-it` preset, can't be empty");t.options&&(this.options={...t.options});let n=t.components;if(n){var r;["core","block","inline"].forEach(i=>{var o;let s=(o=n[i])===null||o===void 0?void 0:o.rules;s&&this[i].ruler.enableOnly(s)});let a=(r=n.inline)===null||r===void 0?void 0:r.rules2;a&&this.inline.ruler2.enableOnly(a)}return this}enable(e,t=!1){let n=[];Array.isArray(e)||(e=[e]),["core","block","inline"].forEach(a=>{n=n.concat(this[a].ruler.enable(e,!0))}),n=n.concat(this.inline.ruler2.enable(e,!0));let r=e.filter(a=>n.indexOf(a)<0);if(r.length&&!t)throw new Error(`MarkdownIt. Failed to enable unknown rule(s): ${r}`);return this}disable(e,t=!1){let n=[];Array.isArray(e)||(e=[e]),["core","block","inline"].forEach(a=>{n=n.concat(this[a].ruler.disable(e,!0))}),n=n.concat(this.inline.ruler2.disable(e,!0));let r=e.filter(a=>n.indexOf(a)<0);if(r.length&&!t)throw new Error(`MarkdownIt. Failed to disable unknown rule(s): ${r}`);return this}use(e,...t){return e.apply(e,[this,...t]),this}parse(e,t){if(typeof e!="string")throw new Error("Input data should be a String");let n=new this.core.State(e,this,t);return this.core.process(n),n.tokens}render(e,t={}){return this.renderer.render(this.parse(e,t),this.options,t)}parseInline(e,t){let n=new this.core.State(e,this,t);return n.inlineMode=!0,this.core.process(n),n.tokens}renderInline(e,t={}){return this.renderer.render(this.parseInline(e,t),this.options,t)}};H($e,"Token",At);H($e,"Ruler",Wn);H($e,"Renderer",Yu);H($e,"ParserCore",Ju);H($e,"StateCore",Ku);H($e,"ParserBlock",rc);H($e,"StateBlock",Qu);H($e,"ParserInline",lc);H($e,"StateInline",ac);var uc=Vu($e);window.BrainSigma={Graph:te,Sigma:ll,MarkdownIt:uc,sdfCircle:Nt,layerFill:cn,layerBorder:ml,pathLine:dn,pathCurved:ul,pathLoop:hn,extremityArrow:cl,layerPlain:Me,forceSimulation:nu,forceLink:tu,forceManyBody:ru,forceCollide:eu,forceX:au,forceY:iu,louvain:cc.default};})();
/*! @sigma/node-border 4.0.0 (MIT)

Copyright (C) 2012-2026, Alexis Jacomy, Guillaume Plique, Benoît Simard https://www.sigmajs.org

Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated documentation files (the "Software"),
to deal in the Software without restriction, including without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense,
and/or sell copies of the Software, and to permit persons to whom the Software is furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS
IN THE SOFTWARE.
*/
/*! d3-dispatch 3.0.2 (ISC)

Copyright 2010-2026 Mike Bostock

Permission to use, copy, modify, and/or distribute this software for any purpose
with or without fee is hereby granted, provided that the above copyright notice
and this permission notice appear in all copies.

THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES WITH
REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF MERCHANTABILITY AND
FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR ANY SPECIAL, DIRECT,
INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES WHATSOEVER RESULTING FROM LOSS
OF USE, DATA OR PROFITS, WHETHER IN AN ACTION OF CONTRACT, NEGLIGENCE OR OTHER
TORTIOUS ACTION, ARISING OUT OF OR IN CONNECTION WITH THE USE OR PERFORMANCE OF
THIS SOFTWARE.
*/
/*! d3-force 3.0.1 (ISC)

Copyright 2010-2026 Mike Bostock

Permission to use, copy, modify, and/or distribute this software for any purpose
with or without fee is hereby granted, provided that the above copyright notice
and this permission notice appear in all copies.

THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES WITH
REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF MERCHANTABILITY AND
FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR ANY SPECIAL, DIRECT,
INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES WHATSOEVER RESULTING FROM LOSS
OF USE, DATA OR PROFITS, WHETHER IN AN ACTION OF CONTRACT, NEGLIGENCE OR OTHER
TORTIOUS ACTION, ARISING OUT OF OR IN CONNECTION WITH THE USE OR PERFORMANCE OF
THIS SOFTWARE.
*/
/*! d3-quadtree 3.0.2 (ISC)

Copyright 2010-2026 Mike Bostock

Permission to use, copy, modify, and/or distribute this software for any purpose
with or without fee is hereby granted, provided that the above copyright notice
and this permission notice appear in all copies.

THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES WITH
REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF MERCHANTABILITY AND
FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR ANY SPECIAL, DIRECT,
INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES WHATSOEVER RESULTING FROM LOSS
OF USE, DATA OR PROFITS, WHETHER IN AN ACTION OF CONTRACT, NEGLIGENCE OR OTHER
TORTIOUS ACTION, ARISING OUT OF OR IN CONNECTION WITH THE USE OR PERFORMANCE OF
THIS SOFTWARE.
*/
/*! d3-timer 3.0.2 (ISC)

Copyright 2010-2026 Mike Bostock

Permission to use, copy, modify, and/or distribute this software for any purpose
with or without fee is hereby granted, provided that the above copyright notice
and this permission notice appear in all copies.

THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES WITH
REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF MERCHANTABILITY AND
FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR ANY SPECIAL, DIRECT,
INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES WHATSOEVER RESULTING FROM LOSS
OF USE, DATA OR PROFITS, WHETHER IN AN ACTION OF CONTRACT, NEGLIGENCE OR OTHER
TORTIOUS ACTION, ARISING OUT OF OR IN CONNECTION WITH THE USE OR PERFORMANCE OF
THIS SOFTWARE.
*/
/*! entities 8.1.0 (BSD-2-Clause)

Copyright (c) Felix Böhm
All rights reserved.

Redistribution and use in source and binary forms, with or without modification, are permitted provided that the following conditions are met:

Redistributions of source code must retain the above copyright notice, this list of conditions and the following disclaimer.

Redistributions in binary form must reproduce the above copyright notice, this list of conditions and the following disclaimer in the documentation and/or other materials provided with the distribution.

THIS IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS,
EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
*/
/*! events 3.3.0 (MIT)

MIT

Copyright Joyent, Inc. and other Node contributors.

Permission is hereby granted, free of charge, to any person obtaining a
copy of this software and associated documentation files (the
"Software"), to deal in the Software without restriction, including
without limitation the rights to use, copy, modify, merge, publish,
distribute, sublicense, and/or sell copies of the Software, and to permit
persons to whom the Software is furnished to do so, subject to the
following conditions:

The above copyright notice and this permission notice shall be included
in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS
OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN
NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM,
DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR
OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE
USE OR OTHER DEALINGS IN THE SOFTWARE.
*/
/*! graphology 0.26.0 (MIT)

The MIT License (MIT)

Copyright (c) 2016-2021 Guillaume Plique (Yomguithereal)

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
THE SOFTWARE.
*/
/*! graphology-communities-louvain 2.0.2 (MIT)

The MIT License (MIT)

Copyright (c) 2017-2021 Guillaume Plique (Yomguithereal)

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
THE SOFTWARE.
*/
/*! graphology-indices 0.17.1 (MIT)

The MIT License (MIT)

Copyright (c) 2020-2021 Guillaume Plique (Yomguithereal)

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
THE SOFTWARE.
*/
/*! graphology-utils 2.5.2 (MIT)

The MIT License (MIT)

Copyright (c) 2017-2021 Guillaume Plique (Yomguithereal)

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
THE SOFTWARE.
*/
/*! linkify-it 6.1.0 (MIT)

Copyright (c) 2015 Vitaly Puzrin.

Permission is hereby granted, free of charge, to any person
obtaining a copy of this software and associated documentation
files (the "Software"), to deal in the Software without
restriction, including without limitation the rights to use,
copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the
Software is furnished to do so, subject to the following
conditions:

The above copyright notice and this permission notice shall be
included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES
OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT
HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY,
WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
OTHER DEALINGS IN THE SOFTWARE.
*/
/*! markdown-it 15.0.3 (MIT)

Copyright (c) 2014 Vitaly Puzrin, Alex Kocharin.

Permission is hereby granted, free of charge, to any person
obtaining a copy of this software and associated documentation
files (the "Software"), to deal in the Software without
restriction, including without limitation the rights to use,
copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the
Software is furnished to do so, subject to the following
conditions:

The above copyright notice and this permission notice shall be
included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES
OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT
HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY,
WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
OTHER DEALINGS IN THE SOFTWARE.
*/
/*! mdurl 2.1.0 (MIT)

Copyright (c) 2015 Vitaly Puzrin, Alex Kocharin.

Permission is hereby granted, free of charge, to any person
obtaining a copy of this software and associated documentation
files (the "Software"), to deal in the Software without
restriction, including without limitation the rights to use,
copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the
Software is furnished to do so, subject to the following
conditions:

The above copyright notice and this permission notice shall be
included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES
OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT
HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY,
WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
OTHER DEALINGS IN THE SOFTWARE.

--------------------------------------------------------------------------------

.parse() is based on Joyent's node.js `url` code:

Copyright Joyent, Inc. and other Node contributors. All rights reserved.
Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to
deal in the Software without restriction, including without limitation the
rights to use, copy, modify, merge, publish, distribute, sublicense, and/or
sell copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS
IN THE SOFTWARE.
*/
/*! mnemonist 0.39.8 (MIT)

The MIT License (MIT)

Copyright (c) 2016 Guillaume Plique (Yomguithereal)

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
THE SOFTWARE.
*/
/*! obliterator 2.0.5 (MIT)

The MIT License (MIT)

Copyright (c) 2017-2021 Guillaume Plique (Yomguithereal)

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
THE SOFTWARE.
*/
/*! pandemonium 2.4.1 (MIT)

The MIT License (MIT)

Copyright (c) 2017-2020 Guillaume Plique (Yomguithereal)

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
THE SOFTWARE.
*/
/*! punycode.js 2.3.1 (MIT)

Copyright Mathias Bynens <https://mathiasbynens.be/>

Permission is hereby granted, free of charge, to any person obtaining
a copy of this software and associated documentation files (the
"Software"), to deal in the Software without restriction, including
without limitation the rights to use, copy, modify, merge, publish,
distribute, sublicense, and/or sell copies of the Software, and to
permit persons to whom the Software is furnished to do so, subject to
the following conditions:

The above copyright notice and this permission notice shall be
included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*/
/*! sigma 4.0.4 (MIT)

Copyright (C) 2012-2026, Alexis Jacomy, Guillaume Plique, Benoît Simard https://www.sigmajs.org

Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated documentation files (the "Software"),
to deal in the Software without restriction, including without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense,
and/or sell copies of the Software, and to permit persons to whom the Software is furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS
IN THE SOFTWARE.
*/
/*! uc.micro 3.0.0 (MIT)

Copyright Mathias Bynens <https://mathiasbynens.be/>

Permission is hereby granted, free of charge, to any person obtaining
a copy of this software and associated documentation files (the
"Software"), to deal in the Software without restriction, including
without limitation the rights to use, copy, modify, merge, publish,
distribute, sublicense, and/or sell copies of the Software, and to
permit persons to whom the Software is furnished to do so, subject to
the following conditions:

The above copyright notice and this permission notice shall be
included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*/
