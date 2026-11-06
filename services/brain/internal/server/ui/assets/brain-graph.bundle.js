/* brain-graph bundle: graphology 0.26.0, graphology-layout 0.6.1, graphology-layout-forceatlas2 0.10.1, graphology-utils 2.5.2, markdown-it 15.0.3, sigma 4.0.4. Built by services/brain/web/build.mjs from web/package-lock.json; licences at the end of this file and in services/brain/THIRD-PARTY.md. Do not edit. */
(()=>{var vu=Object.create;var ar=Object.defineProperty;var mu=Object.getOwnPropertyDescriptor;var bu=Object.getOwnPropertyNames;var yu=Object.getPrototypeOf,_u=Object.prototype.hasOwnProperty;var xu=(e,n,t)=>n in e?ar(e,n,{enumerable:!0,configurable:!0,writable:!0,value:t}):e[n]=t;var Ve=(e,n)=>()=>{try{return n||e((n={exports:{}}).exports,n),n.exports}catch(t){throw n=0,t}},Ti=(e,n)=>{for(var t in n)ar(e,t,{get:n[t],enumerable:!0})},Tu=(e,n,t,r)=>{if(n&&typeof n=="object"||typeof n=="function")for(let a of bu(n))!_u.call(e,a)&&a!==t&&ar(e,a,{get:()=>n[a],enumerable:!(r=mu(n,a))||r.enumerable});return e};var it=(e,n,t)=>(t=e!=null?vu(yu(e)):{},Tu(n||!e||!e.__esModule?ar(t,"default",{value:e,enumerable:!0}):t,e));var xe=(e,n,t)=>xu(e,typeof n!="symbol"?n+"":n,t);var _n=Ve((Hv,sa)=>{"use strict";var qt=typeof Reflect=="object"?Reflect:null,Ei=qt&&typeof qt.apply=="function"?qt.apply:function(n,t,r){return Function.prototype.apply.call(n,t,r)},ir;qt&&typeof qt.ownKeys=="function"?ir=qt.ownKeys:Object.getOwnPropertySymbols?ir=function(n){return Object.getOwnPropertyNames(n).concat(Object.getOwnPropertySymbols(n))}:ir=function(n){return Object.getOwnPropertyNames(n)};function Eu(e){console&&console.warn&&console.warn(e)}var Ri=Number.isNaN||function(n){return n!==n};function ce(){ce.init.call(this)}sa.exports=ce;sa.exports.once=Du;ce.EventEmitter=ce;ce.prototype._events=void 0;ce.prototype._eventsCount=0;ce.prototype._maxListeners=void 0;var Si=10;function or(e){if(typeof e!="function")throw new TypeError('The "listener" argument must be of type Function. Received type '+typeof e)}Object.defineProperty(ce,"defaultMaxListeners",{enumerable:!0,get:function(){return Si},set:function(e){if(typeof e!="number"||e<0||Ri(e))throw new RangeError('The value of "defaultMaxListeners" is out of range. It must be a non-negative number. Received '+e+".");Si=e}});ce.init=function(){(this._events===void 0||this._events===Object.getPrototypeOf(this)._events)&&(this._events=Object.create(null),this._eventsCount=0),this._maxListeners=this._maxListeners||void 0};ce.prototype.setMaxListeners=function(n){if(typeof n!="number"||n<0||Ri(n))throw new RangeError('The value of "n" is out of range. It must be a non-negative number. Received '+n+".");return this._maxListeners=n,this};function Ai(e){return e._maxListeners===void 0?ce.defaultMaxListeners:e._maxListeners}ce.prototype.getMaxListeners=function(){return Ai(this)};ce.prototype.emit=function(n){for(var t=[],r=1;r<arguments.length;r++)t.push(arguments[r]);var a=n==="error",i=this._events;if(i!==void 0)a=a&&i.error===void 0;else if(!a)return!1;if(a){var o;if(t.length>0&&(o=t[0]),o instanceof Error)throw o;var s=new Error("Unhandled error."+(o?" ("+o.message+")":""));throw s.context=o,s}var l=i[n];if(l===void 0)return!1;if(typeof l=="function")Ei(l,this,t);else for(var u=l.length,d=Fi(l,u),r=0;r<u;++r)Ei(d[r],this,t);return!0};function Di(e,n,t,r){var a,i,o;if(or(t),i=e._events,i===void 0?(i=e._events=Object.create(null),e._eventsCount=0):(i.newListener!==void 0&&(e.emit("newListener",n,t.listener?t.listener:t),i=e._events),o=i[n]),o===void 0)o=i[n]=t,++e._eventsCount;else if(typeof o=="function"?o=i[n]=r?[t,o]:[o,t]:r?o.unshift(t):o.push(t),a=Ai(e),a>0&&o.length>a&&!o.warned){o.warned=!0;var s=new Error("Possible EventEmitter memory leak detected. "+o.length+" "+String(n)+" listeners added. Use emitter.setMaxListeners() to increase limit");s.name="MaxListenersExceededWarning",s.emitter=e,s.type=n,s.count=o.length,Eu(s)}return e}ce.prototype.addListener=function(n,t){return Di(this,n,t,!1)};ce.prototype.on=ce.prototype.addListener;ce.prototype.prependListener=function(n,t){return Di(this,n,t,!0)};function Su(){if(!this.fired)return this.target.removeListener(this.type,this.wrapFn),this.fired=!0,arguments.length===0?this.listener.call(this.target):this.listener.apply(this.target,arguments)}function wi(e,n,t){var r={fired:!1,wrapFn:void 0,target:e,type:n,listener:t},a=Su.bind(r);return a.listener=t,r.wrapFn=a,a}ce.prototype.once=function(n,t){return or(t),this.on(n,wi(this,n,t)),this};ce.prototype.prependOnceListener=function(n,t){return or(t),this.prependListener(n,wi(this,n,t)),this};ce.prototype.removeListener=function(n,t){var r,a,i,o,s;if(or(t),a=this._events,a===void 0)return this;if(r=a[n],r===void 0)return this;if(r===t||r.listener===t)--this._eventsCount===0?this._events=Object.create(null):(delete a[n],a.removeListener&&this.emit("removeListener",n,r.listener||t));else if(typeof r!="function"){for(i=-1,o=r.length-1;o>=0;o--)if(r[o]===t||r[o].listener===t){s=r[o].listener,i=o;break}if(i<0)return this;i===0?r.shift():Ru(r,i),r.length===1&&(a[n]=r[0]),a.removeListener!==void 0&&this.emit("removeListener",n,s||t)}return this};ce.prototype.off=ce.prototype.removeListener;ce.prototype.removeAllListeners=function(n){var t,r,a;if(r=this._events,r===void 0)return this;if(r.removeListener===void 0)return arguments.length===0?(this._events=Object.create(null),this._eventsCount=0):r[n]!==void 0&&(--this._eventsCount===0?this._events=Object.create(null):delete r[n]),this;if(arguments.length===0){var i=Object.keys(r),o;for(a=0;a<i.length;++a)o=i[a],o!=="removeListener"&&this.removeAllListeners(o);return this.removeAllListeners("removeListener"),this._events=Object.create(null),this._eventsCount=0,this}if(t=r[n],typeof t=="function")this.removeListener(n,t);else if(t!==void 0)for(a=t.length-1;a>=0;a--)this.removeListener(n,t[a]);return this};function ki(e,n,t){var r=e._events;if(r===void 0)return[];var a=r[n];return a===void 0?[]:typeof a=="function"?t?[a.listener||a]:[a]:t?Au(a):Fi(a,a.length)}ce.prototype.listeners=function(n){return ki(this,n,!0)};ce.prototype.rawListeners=function(n){return ki(this,n,!1)};ce.listenerCount=function(e,n){return typeof e.listenerCount=="function"?e.listenerCount(n):Ci.call(e,n)};ce.prototype.listenerCount=Ci;function Ci(e){var n=this._events;if(n!==void 0){var t=n[e];if(typeof t=="function")return 1;if(t!==void 0)return t.length}return 0}ce.prototype.eventNames=function(){return this._eventsCount>0?ir(this._events):[]};function Fi(e,n){for(var t=new Array(n),r=0;r<n;++r)t[r]=e[r];return t}function Ru(e,n){for(;n+1<e.length;n++)e[n]=e[n+1];e.pop()}function Au(e){for(var n=new Array(e.length),t=0;t<n.length;++t)n[t]=e[t].listener||e[t];return n}function Du(e,n){return new Promise(function(t,r){function a(o){e.removeListener(n,i),r(o)}function i(){typeof e.removeListener=="function"&&e.removeListener("error",a),t([].slice.call(arguments))}Li(e,n,i,{once:!0}),n!=="error"&&wu(e,a,{once:!0})})}function wu(e,n,t){typeof e.on=="function"&&Li(e,"error",n,t)}function Li(e,n,t,r){if(typeof e.on=="function")r.once?e.once(n,t):e.on(n,t);else if(typeof e.addEventListener=="function")e.addEventListener(n,function a(i){r.once&&e.removeEventListener(n,a),t(i)});else throw new TypeError('The "emitter" argument must be of type EventEmitter. Received type '+typeof e)}});var an=Ve((rm,Ao)=>{Ao.exports=function(n){return n!==null&&typeof n=="object"&&typeof n.addUndirectedEdgeWithKey=="function"&&typeof n.dropNode=="function"&&typeof n.multi=="boolean"}});var Za=Ve(qr=>{function og(e){return typeof e!="number"||isNaN(e)?1:e}function sg(e,n){var t={},r=function(o){return typeof o>"u"?n:o};typeof n=="function"&&(r=n);var a=function(o){return r(o[e])},i=function(){return r(void 0)};return typeof e=="string"?(t.fromAttributes=a,t.fromGraph=function(o,s){return a(o.getNodeAttributes(s))},t.fromEntry=function(o,s){return a(s)}):typeof e=="function"?(t.fromAttributes=function(){throw new Error("graphology-utils/getters/createNodeValueGetter: irrelevant usage.")},t.fromGraph=function(o,s){return r(e(s,o.getNodeAttributes(s)))},t.fromEntry=function(o,s){return r(e(o,s))}):(t.fromAttributes=i,t.fromGraph=i,t.fromEntry=i),t}function Us(e,n){var t={},r=function(o){return typeof o>"u"?n:o};typeof n=="function"&&(r=n);var a=function(o){return r(o[e])},i=function(){return r(void 0)};return typeof e=="string"?(t.fromAttributes=a,t.fromGraph=function(o,s){return a(o.getEdgeAttributes(s))},t.fromEntry=function(o,s){return a(s)},t.fromPartialEntry=t.fromEntry,t.fromMinimalEntry=t.fromEntry):typeof e=="function"?(t.fromAttributes=function(){throw new Error("graphology-utils/getters/createEdgeValueGetter: irrelevant usage.")},t.fromGraph=function(o,s){var l=o.extremities(s);return r(e(s,o.getEdgeAttributes(s),l[0],l[1],o.getNodeAttributes(l[0]),o.getNodeAttributes(l[1]),o.isUndirected(s)))},t.fromEntry=function(o,s,l,u,d,c,g){return r(e(o,s,l,u,d,c,g))},t.fromPartialEntry=function(o,s,l,u){return r(e(o,s,l,u))},t.fromMinimalEntry=function(o,s){return r(e(o,s))}):(t.fromAttributes=i,t.fromGraph=i,t.fromEntry=i,t.fromMinimalEntry=i),t}qr.createNodeValueGetter=sg;qr.createEdgeValueGetter=Us;qr.createEdgeWeightGetter=function(e){return Us(e,og)}});var Vs=Ve((Um,$s)=>{var _e=0,ve=1,oe=2,se=3,dt=4,ht=5,ue=6,Os=7,Xr=8,Hs=9,lg=0,ug=1,cg=2,Re=0,He=1,Fe=2,Wt=3,_t=4,me=5,Le=6,et=7,tt=8,js=3,Ye=10,dg=3,Ce=9,Qa=10;$s.exports=function(n,t,r){var a,i,o,s,l,u,d,c,g,h,f=t.length,y=r.length,v=n.adjustSizes,p=n.barnesHutTheta*n.barnesHutTheta,m,b,x,_,T,R,A,S=[];for(o=0;o<f;o+=Ye)t[o+dt]=t[o+oe],t[o+ht]=t[o+se],t[o+oe]=0,t[o+se]=0;if(n.outboundAttractionDistribution){for(m=0,o=0;o<f;o+=Ye)m+=t[o+ue];m/=f/Ye}if(n.barnesHutOptimize){var k=1/0,L=-1/0,C=1/0,z=-1/0,F,P,N;for(o=0;o<f;o+=Ye)k=Math.min(k,t[o+_e]),L=Math.max(L,t[o+_e]),C=Math.min(C,t[o+ve]),z=Math.max(z,t[o+ve]);var G=L-k,E=z-C;for(G>E?(C-=(G-E)/2,z=C+G):(k-=(E-G)/2,L=k+E),S[0+Re]=-1,S[0+He]=(k+L)/2,S[0+Fe]=(C+z)/2,S[0+Wt]=Math.max(L-k,z-C),S[0+_t]=-1,S[0+me]=-1,S[0+Le]=0,S[0+et]=0,S[0+tt]=0,a=1,o=0;o<f;o+=Ye)for(i=0,N=js;;)if(S[i+me]>=0){t[o+_e]<S[i+He]?t[o+ve]<S[i+Fe]?F=S[i+me]:F=S[i+me]+Ce:t[o+ve]<S[i+Fe]?F=S[i+me]+Ce*2:F=S[i+me]+Ce*3,S[i+et]=(S[i+et]*S[i+Le]+t[o+_e]*t[o+ue])/(S[i+Le]+t[o+ue]),S[i+tt]=(S[i+tt]*S[i+Le]+t[o+ve]*t[o+ue])/(S[i+Le]+t[o+ue]),S[i+Le]+=t[o+ue],i=F;continue}else if(S[i+Re]<0){S[i+Re]=o;break}else{if(S[i+me]=a*Ce,c=S[i+Wt]/2,g=S[i+me],S[g+Re]=-1,S[g+He]=S[i+He]-c,S[g+Fe]=S[i+Fe]-c,S[g+Wt]=c,S[g+_t]=g+Ce,S[g+me]=-1,S[g+Le]=0,S[g+et]=0,S[g+tt]=0,g+=Ce,S[g+Re]=-1,S[g+He]=S[i+He]-c,S[g+Fe]=S[i+Fe]+c,S[g+Wt]=c,S[g+_t]=g+Ce,S[g+me]=-1,S[g+Le]=0,S[g+et]=0,S[g+tt]=0,g+=Ce,S[g+Re]=-1,S[g+He]=S[i+He]+c,S[g+Fe]=S[i+Fe]-c,S[g+Wt]=c,S[g+_t]=g+Ce,S[g+me]=-1,S[g+Le]=0,S[g+et]=0,S[g+tt]=0,g+=Ce,S[g+Re]=-1,S[g+He]=S[i+He]+c,S[g+Fe]=S[i+Fe]+c,S[g+Wt]=c,S[g+_t]=S[i+_t],S[g+me]=-1,S[g+Le]=0,S[g+et]=0,S[g+tt]=0,a+=4,t[S[i+Re]+_e]<S[i+He]?t[S[i+Re]+ve]<S[i+Fe]?F=S[i+me]:F=S[i+me]+Ce:t[S[i+Re]+ve]<S[i+Fe]?F=S[i+me]+Ce*2:F=S[i+me]+Ce*3,S[i+Le]=t[S[i+Re]+ue],S[i+et]=t[S[i+Re]+_e],S[i+tt]=t[S[i+Re]+ve],S[F+Re]=S[i+Re],S[i+Re]=-1,t[o+_e]<S[i+He]?t[o+ve]<S[i+Fe]?P=S[i+me]:P=S[i+me]+Ce:t[o+ve]<S[i+Fe]?P=S[i+me]+Ce*2:P=S[i+me]+Ce*3,F===P)if(N--){i=F;continue}else{N=js;break}S[P+Re]=o;break}}if(n.barnesHutOptimize)for(b=n.scalingRatio,o=0;o<f;o+=Ye)for(i=0;;)if(S[i+me]>=0)if(R=Math.pow(t[o+_e]-S[i+et],2)+Math.pow(t[o+ve]-S[i+tt],2),h=S[i+Wt],4*h*h/R<p){if(x=t[o+_e]-S[i+et],_=t[o+ve]-S[i+tt],v===!0?R>0?(A=b*t[o+ue]*S[i+Le]/R,t[o+oe]+=x*A,t[o+se]+=_*A):R<0&&(A=-b*t[o+ue]*S[i+Le]/Math.sqrt(R),t[o+oe]+=x*A,t[o+se]+=_*A):R>0&&(A=b*t[o+ue]*S[i+Le]/R,t[o+oe]+=x*A,t[o+se]+=_*A),i=S[i+_t],i<0)break;continue}else{i=S[i+me];continue}else{if(u=S[i+Re],u>=0&&u!==o&&(x=t[o+_e]-t[u+_e],_=t[o+ve]-t[u+ve],R=x*x+_*_,v===!0?R>0?(A=b*t[o+ue]*t[u+ue]/R,t[o+oe]+=x*A,t[o+se]+=_*A):R<0&&(A=-b*t[o+ue]*t[u+ue]/Math.sqrt(R),t[o+oe]+=x*A,t[o+se]+=_*A):R>0&&(A=b*t[o+ue]*t[u+ue]/R,t[o+oe]+=x*A,t[o+se]+=_*A)),i=S[i+_t],i<0)break;continue}else for(b=n.scalingRatio,s=0;s<f;s+=Ye)for(l=0;l<s;l+=Ye)x=t[s+_e]-t[l+_e],_=t[s+ve]-t[l+ve],v===!0?(R=Math.sqrt(x*x+_*_)-t[s+Xr]-t[l+Xr],R>0?(A=b*t[s+ue]*t[l+ue]/R/R,t[s+oe]+=x*A,t[s+se]+=_*A,t[l+oe]-=x*A,t[l+se]-=_*A):R<0&&(A=100*b*t[s+ue]*t[l+ue],t[s+oe]+=x*A,t[s+se]+=_*A,t[l+oe]-=x*A,t[l+se]-=_*A)):(R=Math.sqrt(x*x+_*_),R>0&&(A=b*t[s+ue]*t[l+ue]/R/R,t[s+oe]+=x*A,t[s+se]+=_*A,t[l+oe]-=x*A,t[l+se]-=_*A));for(g=n.gravity/n.scalingRatio,b=n.scalingRatio,o=0;o<f;o+=Ye)A=0,x=t[o+_e],_=t[o+ve],R=Math.sqrt(Math.pow(x,2)+Math.pow(_,2)),n.strongGravityMode?R>0&&(A=b*t[o+ue]*g):R>0&&(A=b*t[o+ue]*g/R),t[o+oe]-=x*A,t[o+se]-=_*A;for(b=1*(n.outboundAttractionDistribution?m:1),d=0;d<y;d+=dg)s=r[d+lg],l=r[d+ug],c=r[d+cg],T=Math.pow(c,n.edgeWeightInfluence),x=t[s+_e]-t[l+_e],_=t[s+ve]-t[l+ve],v===!0?(R=Math.sqrt(x*x+_*_)-t[s+Xr]-t[l+Xr],n.linLogMode?n.outboundAttractionDistribution?R>0&&(A=-b*T*Math.log(1+R)/R/t[s+ue]):R>0&&(A=-b*T*Math.log(1+R)/R):n.outboundAttractionDistribution?R>0&&(A=-b*T/t[s+ue]):R>0&&(A=-b*T)):(R=Math.sqrt(Math.pow(x,2)+Math.pow(_,2)),n.linLogMode?n.outboundAttractionDistribution?R>0&&(A=-b*T*Math.log(1+R)/R/t[s+ue]):R>0&&(A=-b*T*Math.log(1+R)/R):n.outboundAttractionDistribution?(R=1,A=-b*T/t[s+ue]):(R=1,A=-b*T)),R>0&&(t[s+oe]+=x*A,t[s+se]+=_*A,t[l+oe]-=x*A,t[l+se]-=_*A);var I,W,B,w,$,j;if(v===!0)for(o=0;o<f;o+=Ye)t[o+Hs]!==1&&(I=Math.sqrt(Math.pow(t[o+oe],2)+Math.pow(t[o+se],2)),I>Qa&&(t[o+oe]=t[o+oe]*Qa/I,t[o+se]=t[o+se]*Qa/I),W=t[o+ue]*Math.sqrt((t[o+dt]-t[o+oe])*(t[o+dt]-t[o+oe])+(t[o+ht]-t[o+se])*(t[o+ht]-t[o+se])),B=Math.sqrt((t[o+dt]+t[o+oe])*(t[o+dt]+t[o+oe])+(t[o+ht]+t[o+se])*(t[o+ht]+t[o+se]))/2,w=.1*Math.log(1+B)/(1+Math.sqrt(W)),$=t[o+_e]+t[o+oe]*(w/n.slowDown),t[o+_e]=$,j=t[o+ve]+t[o+se]*(w/n.slowDown),t[o+ve]=j);else for(o=0;o<f;o+=Ye)t[o+Hs]!==1&&(W=t[o+ue]*Math.sqrt((t[o+dt]-t[o+oe])*(t[o+dt]-t[o+oe])+(t[o+ht]-t[o+se])*(t[o+ht]-t[o+se])),B=Math.sqrt((t[o+dt]+t[o+oe])*(t[o+dt]+t[o+oe])+(t[o+ht]+t[o+se])*(t[o+ht]+t[o+se]))/2,w=t[o+Os]*Math.log(1+B)/(1+Math.sqrt(W)),t[o+Os]=Math.min(1,Math.sqrt(w*(Math.pow(t[o+oe],2)+Math.pow(t[o+se],2))/(1+Math.sqrt(W)))),$=t[o+_e]+t[o+oe]*(w/n.slowDown),t[o+_e]=$,j=t[o+ve]+t[o+se]*(w/n.slowDown),t[o+ve]=j);return{}}});var ei=Ve(xt=>{var Wn=10,qs=3;xt.assign=function(e){e=e||{};var n=Array.prototype.slice.call(arguments).slice(1),t,r,a;for(t=0,a=n.length;t<a;t++)if(n[t])for(r in n[t])e[r]=n[t][r];return e};xt.validateSettings=function(e){return"linLogMode"in e&&typeof e.linLogMode!="boolean"?{message:"the `linLogMode` setting should be a boolean."}:"outboundAttractionDistribution"in e&&typeof e.outboundAttractionDistribution!="boolean"?{message:"the `outboundAttractionDistribution` setting should be a boolean."}:"adjustSizes"in e&&typeof e.adjustSizes!="boolean"?{message:"the `adjustSizes` setting should be a boolean."}:"edgeWeightInfluence"in e&&typeof e.edgeWeightInfluence!="number"?{message:"the `edgeWeightInfluence` setting should be a number."}:"scalingRatio"in e&&!(typeof e.scalingRatio=="number"&&e.scalingRatio>=0)?{message:"the `scalingRatio` setting should be a number >= 0."}:"strongGravityMode"in e&&typeof e.strongGravityMode!="boolean"?{message:"the `strongGravityMode` setting should be a boolean."}:"gravity"in e&&!(typeof e.gravity=="number"&&e.gravity>=0)?{message:"the `gravity` setting should be a number >= 0."}:"slowDown"in e&&!(typeof e.slowDown=="number"||e.slowDown>=0)?{message:"the `slowDown` setting should be a number >= 0."}:"barnesHutOptimize"in e&&typeof e.barnesHutOptimize!="boolean"?{message:"the `barnesHutOptimize` setting should be a boolean."}:"barnesHutTheta"in e&&!(typeof e.barnesHutTheta=="number"&&e.barnesHutTheta>=0)?{message:"the `barnesHutTheta` setting should be a number >= 0."}:null};xt.graphToByteArrays=function(e,n){var t=e.order,r=e.size,a={},i,o=new Float32Array(t*Wn),s=new Float32Array(r*qs);return i=0,e.forEachNode(function(l,u){a[l]=i,o[i]=u.x,o[i+1]=u.y,o[i+2]=0,o[i+3]=0,o[i+4]=0,o[i+5]=0,o[i+6]=1,o[i+7]=1,o[i+8]=u.size||1,o[i+9]=u.fixed?1:0,i+=Wn}),i=0,e.forEachEdge(function(l,u,d,c,g,h,f){var y=a[d],v=a[c],p=n(l,u,d,c,g,h,f);o[y+6]+=p,o[v+6]+=p,s[i]=y,s[i+1]=v,s[i+2]=p,i+=qs}),{nodes:o,edges:s}};xt.assignLayoutChanges=function(e,n,t){var r=0;e.updateEachNodeAttributes(function(a,i){return i.x=n[r],i.y=n[r+1],r+=Wn,t?t(a,i):i})};xt.readGraphPositions=function(e,n){var t=0;e.forEachNode(function(r,a){n[t]=a.x,n[t+1]=a.y,t+=Wn})};xt.collectLayoutChanges=function(e,n,t){for(var r=e.nodes(),a={},i=0,o=0,s=n.length;i<s;i+=Wn){if(t){var l=Object.assign({},e.getNodeAttributes(r[o]));l.x=n[i],l.y=n[i+1],l=t(r[o],l),a[r[o]]={x:l.x,y:l.y}}else a[r[o]]={x:n[i],y:n[i+1]};o++}return a};xt.createWorker=function(n){var t=window.URL||window.webkitURL,r=n.toString(),a=t.createObjectURL(new Blob(["("+r+").call(this);"],{type:"text/javascript"})),i=new Worker(a);return t.revokeObjectURL(a),i}});var ti=Ve((Hm,Xs)=>{Xs.exports={linLogMode:!1,outboundAttractionDistribution:!1,adjustSizes:!1,edgeWeightInfluence:1,scalingRatio:1,strongGravityMode:!1,gravity:1,slowDown:1,barnesHutOptimize:!1,barnesHutTheta:.5}});var Ks=Ve((jm,Js)=>{var hg=an(),fg=Za().createEdgeWeightGetter,gg=Vs(),Un=ei(),pg=ti();function Ys(e,n,t){if(!hg(n))throw new Error("graphology-layout-forceatlas2: the given graph is not a valid graphology instance.");typeof t=="number"&&(t={iterations:t});var r=t.iterations;if(typeof r!="number")throw new Error("graphology-layout-forceatlas2: invalid number of iterations.");if(r<=0)throw new Error("graphology-layout-forceatlas2: you should provide a positive number of iterations.");var a=fg("getEdgeWeight"in t?t.getEdgeWeight:"weight").fromEntry,i=typeof t.outputReducer=="function"?t.outputReducer:null,o=Un.assign({},pg,t.settings),s=Un.validateSettings(o);if(s)throw new Error("graphology-layout-forceatlas2: "+s.message);var l=Un.graphToByteArrays(n,a),u;for(u=0;u<r;u++)gg(o,l.nodes,l.edges);if(e){Un.assignLayoutChanges(n,l.nodes,i);return}return Un.collectLayoutChanges(n,l.nodes)}function vg(e){var n=typeof e=="number"?e:e.order;return{barnesHutOptimize:n>2e3,strongGravityMode:!0,gravity:.05,scalingRatio:10,slowDown:1+Math.log(n)}}var ni=Ys.bind(null,!1);ni.assign=Ys.bind(null,!0);ni.inferSettings=vg;Js.exports=ni});var Qs=Ve(($m,Zs)=>{Zs.exports=function(){var n,t,r={};(function(){var i=0,o=1,s=2,l=3,u=4,d=5,c=6,g=7,h=8,f=9,y=0,v=1,p=2,m=0,b=1,x=2,_=3,T=4,R=5,A=6,S=7,k=8,L=3,C=10,z=3,F=9,P=10;r.exports=function(G,E,I){var W,B,w,$,j,J,Se,le,Y,Ke,we=E.length,At=I.length,je=G.adjustSizes,ia=G.barnesHutTheta*G.barnesHutTheta,Ht,fe,ae,ie,$e,Q,X,M=[];for(w=0;w<we;w+=C)E[w+u]=E[w+s],E[w+d]=E[w+l],E[w+s]=0,E[w+l]=0;if(G.outboundAttractionDistribution){for(Ht=0,w=0;w<we;w+=C)Ht+=E[w+c];Ht/=we/C}if(G.barnesHutOptimize){var Dt=1/0,jt=-1/0,wt=1/0,$t=-1/0,ze,Vt,oa;for(w=0;w<we;w+=C)Dt=Math.min(Dt,E[w+i]),jt=Math.max(jt,E[w+i]),wt=Math.min(wt,E[w+o]),$t=Math.max($t,E[w+o]);var Zn=jt-Dt,Qn=$t-wt;for(Zn>Qn?(wt-=(Zn-Qn)/2,$t=wt+Zn):(Dt-=(Qn-Zn)/2,jt=Dt+Qn),M[0+m]=-1,M[0+b]=(Dt+jt)/2,M[0+x]=(wt+$t)/2,M[0+_]=Math.max(jt-Dt,$t-wt),M[0+T]=-1,M[0+R]=-1,M[0+A]=0,M[0+S]=0,M[0+k]=0,W=1,w=0;w<we;w+=C)for(B=0,oa=L;;)if(M[B+R]>=0){E[w+i]<M[B+b]?E[w+o]<M[B+x]?ze=M[B+R]:ze=M[B+R]+F:E[w+o]<M[B+x]?ze=M[B+R]+F*2:ze=M[B+R]+F*3,M[B+S]=(M[B+S]*M[B+A]+E[w+i]*E[w+c])/(M[B+A]+E[w+c]),M[B+k]=(M[B+k]*M[B+A]+E[w+o]*E[w+c])/(M[B+A]+E[w+c]),M[B+A]+=E[w+c],B=ze;continue}else if(M[B+m]<0){M[B+m]=w;break}else{if(M[B+R]=W*F,le=M[B+_]/2,Y=M[B+R],M[Y+m]=-1,M[Y+b]=M[B+b]-le,M[Y+x]=M[B+x]-le,M[Y+_]=le,M[Y+T]=Y+F,M[Y+R]=-1,M[Y+A]=0,M[Y+S]=0,M[Y+k]=0,Y+=F,M[Y+m]=-1,M[Y+b]=M[B+b]-le,M[Y+x]=M[B+x]+le,M[Y+_]=le,M[Y+T]=Y+F,M[Y+R]=-1,M[Y+A]=0,M[Y+S]=0,M[Y+k]=0,Y+=F,M[Y+m]=-1,M[Y+b]=M[B+b]+le,M[Y+x]=M[B+x]-le,M[Y+_]=le,M[Y+T]=Y+F,M[Y+R]=-1,M[Y+A]=0,M[Y+S]=0,M[Y+k]=0,Y+=F,M[Y+m]=-1,M[Y+b]=M[B+b]+le,M[Y+x]=M[B+x]+le,M[Y+_]=le,M[Y+T]=M[B+T],M[Y+R]=-1,M[Y+A]=0,M[Y+S]=0,M[Y+k]=0,W+=4,E[M[B+m]+i]<M[B+b]?E[M[B+m]+o]<M[B+x]?ze=M[B+R]:ze=M[B+R]+F:E[M[B+m]+o]<M[B+x]?ze=M[B+R]+F*2:ze=M[B+R]+F*3,M[B+A]=E[M[B+m]+c],M[B+S]=E[M[B+m]+i],M[B+k]=E[M[B+m]+o],M[ze+m]=M[B+m],M[B+m]=-1,E[w+i]<M[B+b]?E[w+o]<M[B+x]?Vt=M[B+R]:Vt=M[B+R]+F:E[w+o]<M[B+x]?Vt=M[B+R]+F*2:Vt=M[B+R]+F*3,ze===Vt)if(oa--){B=ze;continue}else{oa=L;break}M[Vt+m]=w;break}}if(G.barnesHutOptimize)for(fe=G.scalingRatio,w=0;w<we;w+=C)for(B=0;;)if(M[B+R]>=0)if(Q=Math.pow(E[w+i]-M[B+S],2)+Math.pow(E[w+o]-M[B+k],2),Ke=M[B+_],4*Ke*Ke/Q<ia){if(ae=E[w+i]-M[B+S],ie=E[w+o]-M[B+k],je===!0?Q>0?(X=fe*E[w+c]*M[B+A]/Q,E[w+s]+=ae*X,E[w+l]+=ie*X):Q<0&&(X=-fe*E[w+c]*M[B+A]/Math.sqrt(Q),E[w+s]+=ae*X,E[w+l]+=ie*X):Q>0&&(X=fe*E[w+c]*M[B+A]/Q,E[w+s]+=ae*X,E[w+l]+=ie*X),B=M[B+T],B<0)break;continue}else{B=M[B+R];continue}else{if(J=M[B+m],J>=0&&J!==w&&(ae=E[w+i]-E[J+i],ie=E[w+o]-E[J+o],Q=ae*ae+ie*ie,je===!0?Q>0?(X=fe*E[w+c]*E[J+c]/Q,E[w+s]+=ae*X,E[w+l]+=ie*X):Q<0&&(X=-fe*E[w+c]*E[J+c]/Math.sqrt(Q),E[w+s]+=ae*X,E[w+l]+=ie*X):Q>0&&(X=fe*E[w+c]*E[J+c]/Q,E[w+s]+=ae*X,E[w+l]+=ie*X)),B=M[B+T],B<0)break;continue}else for(fe=G.scalingRatio,$=0;$<we;$+=C)for(j=0;j<$;j+=C)ae=E[$+i]-E[j+i],ie=E[$+o]-E[j+o],je===!0?(Q=Math.sqrt(ae*ae+ie*ie)-E[$+h]-E[j+h],Q>0?(X=fe*E[$+c]*E[j+c]/Q/Q,E[$+s]+=ae*X,E[$+l]+=ie*X,E[j+s]-=ae*X,E[j+l]-=ie*X):Q<0&&(X=100*fe*E[$+c]*E[j+c],E[$+s]+=ae*X,E[$+l]+=ie*X,E[j+s]-=ae*X,E[j+l]-=ie*X)):(Q=Math.sqrt(ae*ae+ie*ie),Q>0&&(X=fe*E[$+c]*E[j+c]/Q/Q,E[$+s]+=ae*X,E[$+l]+=ie*X,E[j+s]-=ae*X,E[j+l]-=ie*X));for(Y=G.gravity/G.scalingRatio,fe=G.scalingRatio,w=0;w<we;w+=C)X=0,ae=E[w+i],ie=E[w+o],Q=Math.sqrt(Math.pow(ae,2)+Math.pow(ie,2)),G.strongGravityMode?Q>0&&(X=fe*E[w+c]*Y):Q>0&&(X=fe*E[w+c]*Y/Q),E[w+s]-=ae*X,E[w+l]-=ie*X;for(fe=1*(G.outboundAttractionDistribution?Ht:1),Se=0;Se<At;Se+=z)$=I[Se+y],j=I[Se+v],le=I[Se+p],$e=Math.pow(le,G.edgeWeightInfluence),ae=E[$+i]-E[j+i],ie=E[$+o]-E[j+o],je===!0?(Q=Math.sqrt(ae*ae+ie*ie)-E[$+h]-E[j+h],G.linLogMode?G.outboundAttractionDistribution?Q>0&&(X=-fe*$e*Math.log(1+Q)/Q/E[$+c]):Q>0&&(X=-fe*$e*Math.log(1+Q)/Q):G.outboundAttractionDistribution?Q>0&&(X=-fe*$e/E[$+c]):Q>0&&(X=-fe*$e)):(Q=Math.sqrt(Math.pow(ae,2)+Math.pow(ie,2)),G.linLogMode?G.outboundAttractionDistribution?Q>0&&(X=-fe*$e*Math.log(1+Q)/Q/E[$+c]):Q>0&&(X=-fe*$e*Math.log(1+Q)/Q):G.outboundAttractionDistribution?(Q=1,X=-fe*$e/E[$+c]):(Q=1,X=-fe*$e)),Q>0&&(E[$+s]+=ae*X,E[$+l]+=ie*X,E[j+s]-=ae*X,E[j+l]-=ie*X);var er,yn,tr,kt,nr,rr;if(je===!0)for(w=0;w<we;w+=C)E[w+f]!==1&&(er=Math.sqrt(Math.pow(E[w+s],2)+Math.pow(E[w+l],2)),er>P&&(E[w+s]=E[w+s]*P/er,E[w+l]=E[w+l]*P/er),yn=E[w+c]*Math.sqrt((E[w+u]-E[w+s])*(E[w+u]-E[w+s])+(E[w+d]-E[w+l])*(E[w+d]-E[w+l])),tr=Math.sqrt((E[w+u]+E[w+s])*(E[w+u]+E[w+s])+(E[w+d]+E[w+l])*(E[w+d]+E[w+l]))/2,kt=.1*Math.log(1+tr)/(1+Math.sqrt(yn)),nr=E[w+i]+E[w+s]*(kt/G.slowDown),E[w+i]=nr,rr=E[w+o]+E[w+l]*(kt/G.slowDown),E[w+o]=rr);else for(w=0;w<we;w+=C)E[w+f]!==1&&(yn=E[w+c]*Math.sqrt((E[w+u]-E[w+s])*(E[w+u]-E[w+s])+(E[w+d]-E[w+l])*(E[w+d]-E[w+l])),tr=Math.sqrt((E[w+u]+E[w+s])*(E[w+u]+E[w+s])+(E[w+d]+E[w+l])*(E[w+d]+E[w+l]))/2,kt=E[w+g]*Math.log(1+tr)/(1+Math.sqrt(yn)),E[w+g]=Math.min(1,Math.sqrt(kt*(Math.pow(E[w+s],2)+Math.pow(E[w+l],2))/(1+Math.sqrt(yn)))),nr=E[w+i]+E[w+s]*(kt/G.slowDown),E[w+i]=nr,rr=E[w+o]+E[w+l]*(kt/G.slowDown),E[w+o]=rr);return{}}})();var a=r.exports;self.addEventListener("message",function(i){var o=i.data;n=new Float32Array(o.nodes),o.edges&&(t=new Float32Array(o.edges)),a(o.settings,n,t),self.postMessage({nodes:n.buffer},[n.buffer])})}});var tl=Ve((Vm,el)=>{var mg=Qs(),bg=an(),yg=Za().createEdgeWeightGetter,vn=ei(),_g=ti();function Tt(e,n){if(n=n||{},!bg(e))throw new Error("graphology-layout-forceatlas2/worker: the given graph is not a valid graphology instance.");var t=yg("getEdgeWeight"in n?n.getEdgeWeight:"weight").fromEntry,r=vn.assign({},_g,n.settings),a=vn.validateSettings(r);if(a)throw new Error("graphology-layout-forceatlas2/worker: "+a.message);this.worker=null,this.graph=e,this.settings=r,this.getEdgeWeight=t,this.matrices=null,this.running=!1,this.killed=!1,this.outputReducer=typeof n.outputReducer=="function"?n.outputReducer:null,this.handleMessage=this.handleMessage.bind(this);var i=void 0,o=this;this.handleGraphUpdate=function(){o.worker&&o.worker.terminate(),i&&clearTimeout(i),i=setTimeout(function(){i=void 0,o.spawnWorker()},0)},e.on("nodeAdded",this.handleGraphUpdate),e.on("edgeAdded",this.handleGraphUpdate),e.on("nodeDropped",this.handleGraphUpdate),e.on("edgeDropped",this.handleGraphUpdate),this.spawnWorker()}Tt.prototype.isRunning=function(){return this.running};Tt.prototype.spawnWorker=function(){this.worker&&this.worker.terminate(),this.worker=vn.createWorker(mg),this.worker.addEventListener("message",this.handleMessage),this.running&&(this.running=!1,this.start())};Tt.prototype.handleMessage=function(e){if(this.running){var n=new Float32Array(e.data.nodes);vn.assignLayoutChanges(this.graph,n,this.outputReducer),this.outputReducer&&vn.readGraphPositions(this.graph,n),this.matrices.nodes=n,this.askForIterations()}};Tt.prototype.askForIterations=function(e){var n=this.matrices,t={settings:this.settings,nodes:n.nodes.buffer},r=[n.nodes.buffer];return e&&(t.edges=n.edges.buffer,r.push(n.edges.buffer)),this.worker.postMessage(t,r),this};Tt.prototype.start=function(){if(this.killed)throw new Error("graphology-layout-forceatlas2/worker.start: layout was killed.");return this.running?this:(this.matrices=vn.graphToByteArrays(this.graph,this.getEdgeWeight),this.running=!0,this.askForIterations(!0),this)};Tt.prototype.stop=function(){return this.running=!1,this};Tt.prototype.kill=function(){if(this.killed)return this;this.running=!1,this.killed=!0,this.matrices=null,this.worker.terminate(),this.graph.removeListener("nodeAdded",this.handleGraphUpdate),this.graph.removeListener("edgeAdded",this.handleGraphUpdate),this.graph.removeListener("nodeDropped",this.handleGraphUpdate),this.graph.removeListener("edgeDropped",this.handleGraphUpdate)};el.exports=Tt});var al=Ve((qm,rl)=>{function xg(e){return!e||typeof e!="object"||typeof e=="function"||Array.isArray(e)||e instanceof Set||e instanceof Map||e instanceof RegExp||e instanceof Date}function nl(e,n){e=e||{};var t={};for(var r in n){var a=e[r],i=n[r];if(!xg(i)){t[r]=nl(a,i);continue}a===void 0?t[r]=i:t[r]=a}return t}rl.exports=nl});var ll=Ve((Xm,sl)=>{var Tg=al(),Eg=an(),Sg={dimensions:["x","y"],center:.5,scale:1};function il(e,n,t){if(!Eg(n))throw new Error("graphology-layout/random: the given graph is not a valid graphology instance.");t=Tg(t,Sg);var r=t.dimensions;if(!Array.isArray(r)||r.length!==2)throw new Error("graphology-layout/random: given dimensions are invalid.");var a=t.center,i=t.scale,o=Math.PI*2,s=(a-.5)*i,l=n.order,u=r[0],d=r[1];function c(f,y){return y[u]=i*Math.cos(f*o/l)+s,y[d]=i*Math.sin(f*o/l)+s,y}var g=0;if(!e){var h={};return n.forEachNode(function(f){h[f]=c(g++,{})}),h}n.updateEachNodeAttributes(function(f,y){return c(g++,y),y},{attributes:r})}var ol=il.bind(null,!1);ol.assign=il.bind(null,!0);sl.exports=ol});var zi=it(_n(),1);function ku(){let e=arguments[0];for(let n=1,t=arguments.length;n<t;n++)if(arguments[n])for(let r in arguments[n])e[r]=arguments[n][r];return e}var Te=ku;typeof Object.assign=="function"&&(Te=Object.assign);function Ne(e,n,t,r){let a=e._nodes.get(n),i=null;return a&&(r==="mixed"?i=a.out&&a.out[t]||a.undirected&&a.undirected[t]:r==="directed"?i=a.out&&a.out[t]:i=a.undirected&&a.undirected[t]),i}function Ae(e){return typeof e=="object"&&e!==null}function Bi(e){let n;for(n in e)return!1;return!0}function Be(e,n,t){Object.defineProperty(e,n,{enumerable:!1,configurable:!1,writable:!0,value:t})}function qe(e,n,t){let r={enumerable:!0,configurable:!0};typeof t=="function"?r.get=t:(r.value=t,r.writable=!1),Object.defineProperty(e,n,r)}function Pi(e){return!(!Ae(e)||e.attributes&&!Array.isArray(e.attributes))}function Cu(){let e=Math.floor(Math.random()*256)&255;return()=>e++}function ot(){let e=arguments,n=null,t=-1;return{[Symbol.iterator](){return this},next(){let r=null;do{if(n===null){if(t++,t>=e.length)return{done:!0};n=e[t][Symbol.iterator]()}if(r=n.next(),r.done){n=null;continue}break}while(!0);return r}}}function Yt(){return{[Symbol.iterator](){return this},next(){return{done:!0}}}}var En=class extends Error{constructor(n){super(),this.name="GraphError",this.message=n}},V=class e extends En{constructor(n){super(n),this.name="InvalidArgumentsGraphError",typeof Error.captureStackTrace=="function"&&Error.captureStackTrace(this,e.prototype.constructor)}},H=class e extends En{constructor(n){super(n),this.name="NotFoundGraphError",typeof Error.captureStackTrace=="function"&&Error.captureStackTrace(this,e.prototype.constructor)}},Z=class e extends En{constructor(n){super(n),this.name="UsageGraphError",typeof Error.captureStackTrace=="function"&&Error.captureStackTrace(this,e.prototype.constructor)}};function Ni(e,n){this.key=e,this.attributes=n,this.clear()}Ni.prototype.clear=function(){this.inDegree=0,this.outDegree=0,this.undirectedDegree=0,this.undirectedLoops=0,this.directedLoops=0,this.in={},this.out={},this.undirected={}};function Mi(e,n){this.key=e,this.attributes=n,this.clear()}Mi.prototype.clear=function(){this.inDegree=0,this.outDegree=0,this.directedLoops=0,this.in={},this.out={}};function Wi(e,n){this.key=e,this.attributes=n,this.clear()}Wi.prototype.clear=function(){this.undirectedDegree=0,this.undirectedLoops=0,this.undirected={}};function Jt(e,n,t,r,a){this.key=n,this.attributes=a,this.undirected=e,this.source=t,this.target=r}Jt.prototype.attach=function(){let e="out",n="in";this.undirected&&(e=n="undirected");let t=this.source.key,r=this.target.key;this.source[e][r]=this,!(this.undirected&&t===r)&&(this.target[n][t]=this)};Jt.prototype.attachMulti=function(){let e="out",n="in",t=this.source.key,r=this.target.key;this.undirected&&(e=n="undirected");let a=this.source[e],i=a[r];if(typeof i>"u"){a[r]=this,this.undirected&&t===r||(this.target[n][t]=this);return}i.previous=this,this.next=i,a[r]=this,this.target[n][t]=this};Jt.prototype.detach=function(){let e=this.source.key,n=this.target.key,t="out",r="in";this.undirected&&(t=r="undirected"),delete this.source[t][n],delete this.target[r][e]};Jt.prototype.detachMulti=function(){let e=this.source.key,n=this.target.key,t="out",r="in";this.undirected&&(t=r="undirected"),this.previous===void 0?this.next===void 0?(delete this.source[t][n],delete this.target[r][e]):(this.next.previous=void 0,this.source[t][n]=this.next,this.target[r][e]=this.next):(this.previous.next=this.next,this.next!==void 0&&(this.next.previous=this.previous))};var Ui=0,Oi=1,Fu=2,Hi=3;function st(e,n,t,r,a,i,o){let s,l,u,d;if(r=""+r,t===Ui){if(s=e._nodes.get(r),!s)throw new H(`Graph.${n}: could not find the "${r}" node in the graph.`);u=a,d=i}else if(t===Hi){if(a=""+a,l=e._edges.get(a),!l)throw new H(`Graph.${n}: could not find the "${a}" edge in the graph.`);let c=l.source.key,g=l.target.key;if(r===c)s=l.target;else if(r===g)s=l.source;else throw new H(`Graph.${n}: the "${r}" node is not attached to the "${a}" edge (${c}, ${g}).`);u=i,d=o}else{if(l=e._edges.get(r),!l)throw new H(`Graph.${n}: could not find the "${r}" edge in the graph.`);t===Oi?s=l.source:s=l.target,u=a,d=i}return[s,u,d]}function Lu(e,n,t){e.prototype[n]=function(r,a,i){let[o,s]=st(this,n,t,r,a,i);return o.attributes[s]}}function Pu(e,n,t){e.prototype[n]=function(r,a){let[i]=st(this,n,t,r,a);return i.attributes}}function Iu(e,n,t){e.prototype[n]=function(r,a,i){let[o,s]=st(this,n,t,r,a,i);return o.attributes.hasOwnProperty(s)}}function Gu(e,n,t){e.prototype[n]=function(r,a,i,o){let[s,l,u]=st(this,n,t,r,a,i,o);return s.attributes[l]=u,this.emit("nodeAttributesUpdated",{key:s.key,type:"set",attributes:s.attributes,name:l}),this}}function zu(e,n,t){e.prototype[n]=function(r,a,i,o){let[s,l,u]=st(this,n,t,r,a,i,o);if(typeof u!="function")throw new V(`Graph.${n}: updater should be a function.`);let d=s.attributes,c=u(d[l]);return d[l]=c,this.emit("nodeAttributesUpdated",{key:s.key,type:"set",attributes:s.attributes,name:l}),this}}function Bu(e,n,t){e.prototype[n]=function(r,a,i){let[o,s]=st(this,n,t,r,a,i);return delete o.attributes[s],this.emit("nodeAttributesUpdated",{key:o.key,type:"remove",attributes:o.attributes,name:s}),this}}function Nu(e,n,t){e.prototype[n]=function(r,a,i){let[o,s]=st(this,n,t,r,a,i);if(!Ae(s))throw new V(`Graph.${n}: provided attributes are not a plain object.`);return o.attributes=s,this.emit("nodeAttributesUpdated",{key:o.key,type:"replace",attributes:o.attributes}),this}}function Mu(e,n,t){e.prototype[n]=function(r,a,i){let[o,s]=st(this,n,t,r,a,i);if(!Ae(s))throw new V(`Graph.${n}: provided attributes are not a plain object.`);return Te(o.attributes,s),this.emit("nodeAttributesUpdated",{key:o.key,type:"merge",attributes:o.attributes,data:s}),this}}function Wu(e,n,t){e.prototype[n]=function(r,a,i){let[o,s]=st(this,n,t,r,a,i);if(typeof s!="function")throw new V(`Graph.${n}: provided updater is not a function.`);return o.attributes=s(o.attributes),this.emit("nodeAttributesUpdated",{key:o.key,type:"update",attributes:o.attributes}),this}}var Uu=[{name:e=>`get${e}Attribute`,attacher:Lu},{name:e=>`get${e}Attributes`,attacher:Pu},{name:e=>`has${e}Attribute`,attacher:Iu},{name:e=>`set${e}Attribute`,attacher:Gu},{name:e=>`update${e}Attribute`,attacher:zu},{name:e=>`remove${e}Attribute`,attacher:Bu},{name:e=>`replace${e}Attributes`,attacher:Nu},{name:e=>`merge${e}Attributes`,attacher:Mu},{name:e=>`update${e}Attributes`,attacher:Wu}];function Ou(e){Uu.forEach(function({name:n,attacher:t}){t(e,n("Node"),Ui),t(e,n("Source"),Oi),t(e,n("Target"),Fu),t(e,n("Opposite"),Hi)})}function Hu(e,n,t){e.prototype[n]=function(r,a){let i;if(this.type!=="mixed"&&t!=="mixed"&&t!==this.type)throw new Z(`Graph.${n}: cannot find this type of edges in your ${this.type} graph.`);if(arguments.length>2){if(this.multi)throw new Z(`Graph.${n}: cannot use a {source,target} combo when asking about an edge's attributes in a MultiGraph since we cannot infer the one you want information about.`);let o=""+r,s=""+a;if(a=arguments[2],i=Ne(this,o,s,t),!i)throw new H(`Graph.${n}: could not find an edge for the given path ("${o}" - "${s}").`)}else{if(t!=="mixed")throw new Z(`Graph.${n}: calling this method with only a key (vs. a source and target) does not make sense since an edge with this key could have the other type.`);if(r=""+r,i=this._edges.get(r),!i)throw new H(`Graph.${n}: could not find the "${r}" edge in the graph.`)}return i.attributes[a]}}function ju(e,n,t){e.prototype[n]=function(r){let a;if(this.type!=="mixed"&&t!=="mixed"&&t!==this.type)throw new Z(`Graph.${n}: cannot find this type of edges in your ${this.type} graph.`);if(arguments.length>1){if(this.multi)throw new Z(`Graph.${n}: cannot use a {source,target} combo when asking about an edge's attributes in a MultiGraph since we cannot infer the one you want information about.`);let i=""+r,o=""+arguments[1];if(a=Ne(this,i,o,t),!a)throw new H(`Graph.${n}: could not find an edge for the given path ("${i}" - "${o}").`)}else{if(t!=="mixed")throw new Z(`Graph.${n}: calling this method with only a key (vs. a source and target) does not make sense since an edge with this key could have the other type.`);if(r=""+r,a=this._edges.get(r),!a)throw new H(`Graph.${n}: could not find the "${r}" edge in the graph.`)}return a.attributes}}function $u(e,n,t){e.prototype[n]=function(r,a){let i;if(this.type!=="mixed"&&t!=="mixed"&&t!==this.type)throw new Z(`Graph.${n}: cannot find this type of edges in your ${this.type} graph.`);if(arguments.length>2){if(this.multi)throw new Z(`Graph.${n}: cannot use a {source,target} combo when asking about an edge's attributes in a MultiGraph since we cannot infer the one you want information about.`);let o=""+r,s=""+a;if(a=arguments[2],i=Ne(this,o,s,t),!i)throw new H(`Graph.${n}: could not find an edge for the given path ("${o}" - "${s}").`)}else{if(t!=="mixed")throw new Z(`Graph.${n}: calling this method with only a key (vs. a source and target) does not make sense since an edge with this key could have the other type.`);if(r=""+r,i=this._edges.get(r),!i)throw new H(`Graph.${n}: could not find the "${r}" edge in the graph.`)}return i.attributes.hasOwnProperty(a)}}function Vu(e,n,t){e.prototype[n]=function(r,a,i){let o;if(this.type!=="mixed"&&t!=="mixed"&&t!==this.type)throw new Z(`Graph.${n}: cannot find this type of edges in your ${this.type} graph.`);if(arguments.length>3){if(this.multi)throw new Z(`Graph.${n}: cannot use a {source,target} combo when asking about an edge's attributes in a MultiGraph since we cannot infer the one you want information about.`);let s=""+r,l=""+a;if(a=arguments[2],i=arguments[3],o=Ne(this,s,l,t),!o)throw new H(`Graph.${n}: could not find an edge for the given path ("${s}" - "${l}").`)}else{if(t!=="mixed")throw new Z(`Graph.${n}: calling this method with only a key (vs. a source and target) does not make sense since an edge with this key could have the other type.`);if(r=""+r,o=this._edges.get(r),!o)throw new H(`Graph.${n}: could not find the "${r}" edge in the graph.`)}return o.attributes[a]=i,this.emit("edgeAttributesUpdated",{key:o.key,type:"set",attributes:o.attributes,name:a}),this}}function qu(e,n,t){e.prototype[n]=function(r,a,i){let o;if(this.type!=="mixed"&&t!=="mixed"&&t!==this.type)throw new Z(`Graph.${n}: cannot find this type of edges in your ${this.type} graph.`);if(arguments.length>3){if(this.multi)throw new Z(`Graph.${n}: cannot use a {source,target} combo when asking about an edge's attributes in a MultiGraph since we cannot infer the one you want information about.`);let s=""+r,l=""+a;if(a=arguments[2],i=arguments[3],o=Ne(this,s,l,t),!o)throw new H(`Graph.${n}: could not find an edge for the given path ("${s}" - "${l}").`)}else{if(t!=="mixed")throw new Z(`Graph.${n}: calling this method with only a key (vs. a source and target) does not make sense since an edge with this key could have the other type.`);if(r=""+r,o=this._edges.get(r),!o)throw new H(`Graph.${n}: could not find the "${r}" edge in the graph.`)}if(typeof i!="function")throw new V(`Graph.${n}: updater should be a function.`);return o.attributes[a]=i(o.attributes[a]),this.emit("edgeAttributesUpdated",{key:o.key,type:"set",attributes:o.attributes,name:a}),this}}function Xu(e,n,t){e.prototype[n]=function(r,a){let i;if(this.type!=="mixed"&&t!=="mixed"&&t!==this.type)throw new Z(`Graph.${n}: cannot find this type of edges in your ${this.type} graph.`);if(arguments.length>2){if(this.multi)throw new Z(`Graph.${n}: cannot use a {source,target} combo when asking about an edge's attributes in a MultiGraph since we cannot infer the one you want information about.`);let o=""+r,s=""+a;if(a=arguments[2],i=Ne(this,o,s,t),!i)throw new H(`Graph.${n}: could not find an edge for the given path ("${o}" - "${s}").`)}else{if(t!=="mixed")throw new Z(`Graph.${n}: calling this method with only a key (vs. a source and target) does not make sense since an edge with this key could have the other type.`);if(r=""+r,i=this._edges.get(r),!i)throw new H(`Graph.${n}: could not find the "${r}" edge in the graph.`)}return delete i.attributes[a],this.emit("edgeAttributesUpdated",{key:i.key,type:"remove",attributes:i.attributes,name:a}),this}}function Yu(e,n,t){e.prototype[n]=function(r,a){let i;if(this.type!=="mixed"&&t!=="mixed"&&t!==this.type)throw new Z(`Graph.${n}: cannot find this type of edges in your ${this.type} graph.`);if(arguments.length>2){if(this.multi)throw new Z(`Graph.${n}: cannot use a {source,target} combo when asking about an edge's attributes in a MultiGraph since we cannot infer the one you want information about.`);let o=""+r,s=""+a;if(a=arguments[2],i=Ne(this,o,s,t),!i)throw new H(`Graph.${n}: could not find an edge for the given path ("${o}" - "${s}").`)}else{if(t!=="mixed")throw new Z(`Graph.${n}: calling this method with only a key (vs. a source and target) does not make sense since an edge with this key could have the other type.`);if(r=""+r,i=this._edges.get(r),!i)throw new H(`Graph.${n}: could not find the "${r}" edge in the graph.`)}if(!Ae(a))throw new V(`Graph.${n}: provided attributes are not a plain object.`);return i.attributes=a,this.emit("edgeAttributesUpdated",{key:i.key,type:"replace",attributes:i.attributes}),this}}function Ju(e,n,t){e.prototype[n]=function(r,a){let i;if(this.type!=="mixed"&&t!=="mixed"&&t!==this.type)throw new Z(`Graph.${n}: cannot find this type of edges in your ${this.type} graph.`);if(arguments.length>2){if(this.multi)throw new Z(`Graph.${n}: cannot use a {source,target} combo when asking about an edge's attributes in a MultiGraph since we cannot infer the one you want information about.`);let o=""+r,s=""+a;if(a=arguments[2],i=Ne(this,o,s,t),!i)throw new H(`Graph.${n}: could not find an edge for the given path ("${o}" - "${s}").`)}else{if(t!=="mixed")throw new Z(`Graph.${n}: calling this method with only a key (vs. a source and target) does not make sense since an edge with this key could have the other type.`);if(r=""+r,i=this._edges.get(r),!i)throw new H(`Graph.${n}: could not find the "${r}" edge in the graph.`)}if(!Ae(a))throw new V(`Graph.${n}: provided attributes are not a plain object.`);return Te(i.attributes,a),this.emit("edgeAttributesUpdated",{key:i.key,type:"merge",attributes:i.attributes,data:a}),this}}function Ku(e,n,t){e.prototype[n]=function(r,a){let i;if(this.type!=="mixed"&&t!=="mixed"&&t!==this.type)throw new Z(`Graph.${n}: cannot find this type of edges in your ${this.type} graph.`);if(arguments.length>2){if(this.multi)throw new Z(`Graph.${n}: cannot use a {source,target} combo when asking about an edge's attributes in a MultiGraph since we cannot infer the one you want information about.`);let o=""+r,s=""+a;if(a=arguments[2],i=Ne(this,o,s,t),!i)throw new H(`Graph.${n}: could not find an edge for the given path ("${o}" - "${s}").`)}else{if(t!=="mixed")throw new Z(`Graph.${n}: calling this method with only a key (vs. a source and target) does not make sense since an edge with this key could have the other type.`);if(r=""+r,i=this._edges.get(r),!i)throw new H(`Graph.${n}: could not find the "${r}" edge in the graph.`)}if(typeof a!="function")throw new V(`Graph.${n}: provided updater is not a function.`);return i.attributes=a(i.attributes),this.emit("edgeAttributesUpdated",{key:i.key,type:"update",attributes:i.attributes}),this}}var Zu=[{name:e=>`get${e}Attribute`,attacher:Hu},{name:e=>`get${e}Attributes`,attacher:ju},{name:e=>`has${e}Attribute`,attacher:$u},{name:e=>`set${e}Attribute`,attacher:Vu},{name:e=>`update${e}Attribute`,attacher:qu},{name:e=>`remove${e}Attribute`,attacher:Xu},{name:e=>`replace${e}Attributes`,attacher:Yu},{name:e=>`merge${e}Attributes`,attacher:Ju},{name:e=>`update${e}Attributes`,attacher:Ku}];function Qu(e){Zu.forEach(function({name:n,attacher:t}){t(e,n("Edge"),"mixed"),t(e,n("DirectedEdge"),"directed"),t(e,n("UndirectedEdge"),"undirected")})}var ec=[{name:"edges",type:"mixed"},{name:"inEdges",type:"directed",direction:"in"},{name:"outEdges",type:"directed",direction:"out"},{name:"inboundEdges",type:"mixed",direction:"in"},{name:"outboundEdges",type:"mixed",direction:"out"},{name:"directedEdges",type:"directed"},{name:"undirectedEdges",type:"undirected"}];function tc(e,n,t,r){let a=!1;for(let i in n){if(i===r)continue;let o=n[i];if(a=t(o.key,o.attributes,o.source.key,o.target.key,o.source.attributes,o.target.attributes,o.undirected),e&&a)return o.key}}function nc(e,n,t,r){let a,i,o,s=!1;for(let l in n)if(l!==r){a=n[l];do{if(i=a.source,o=a.target,s=t(a.key,a.attributes,i.key,o.key,i.attributes,o.attributes,a.undirected),e&&s)return a.key;a=a.next}while(a!==void 0)}}function la(e,n){let t=Object.keys(e),r=t.length,a,i=0;return{[Symbol.iterator](){return this},next(){do if(a)a=a.next;else{if(i>=r)return{done:!0};let o=t[i++];if(o===n){a=void 0;continue}a=e[o]}while(!a);return{done:!1,value:{edge:a.key,attributes:a.attributes,source:a.source.key,target:a.target.key,sourceAttributes:a.source.attributes,targetAttributes:a.target.attributes,undirected:a.undirected}}}}}function rc(e,n,t,r){let a=n[t];if(!a)return;let i=a.source,o=a.target;if(r(a.key,a.attributes,i.key,o.key,i.attributes,o.attributes,a.undirected)&&e)return a.key}function ac(e,n,t,r){let a=n[t];if(!a)return;let i=!1;do{if(i=r(a.key,a.attributes,a.source.key,a.target.key,a.source.attributes,a.target.attributes,a.undirected),e&&i)return a.key;a=a.next}while(a!==void 0)}function ua(e,n){let t=e[n];if(t.next!==void 0)return{[Symbol.iterator](){return this},next(){if(!t)return{done:!0};let a={edge:t.key,attributes:t.attributes,source:t.source.key,target:t.target.key,sourceAttributes:t.source.attributes,targetAttributes:t.target.attributes,undirected:t.undirected};return t=t.next,{done:!1,value:a}}};let r=!1;return{[Symbol.iterator](){return this},next(){return r===!0?{done:!0}:(r=!0,{done:!1,value:{edge:t.key,attributes:t.attributes,source:t.source.key,target:t.target.key,sourceAttributes:t.source.attributes,targetAttributes:t.target.attributes,undirected:t.undirected}})}}}function ic(e,n){if(e.size===0)return[];if(n==="mixed"||n===e.type)return Array.from(e._edges.keys());let t=n==="undirected"?e.undirectedSize:e.directedSize,r=new Array(t),a=n==="undirected",i=e._edges.values(),o=0,s,l;for(;s=i.next(),s.done!==!0;)l=s.value,l.undirected===a&&(r[o++]=l.key);return r}function ji(e,n,t,r){if(n.size===0)return;let a=t!=="mixed"&&t!==n.type,i=t==="undirected",o,s,l=!1,u=n._edges.values();for(;o=u.next(),o.done!==!0;){if(s=o.value,a&&s.undirected!==i)continue;let{key:d,attributes:c,source:g,target:h}=s;if(l=r(d,c,g.key,h.key,g.attributes,h.attributes,s.undirected),e&&l)return d}}function oc(e,n){if(e.size===0)return Yt();let t=n!=="mixed"&&n!==e.type,r=n==="undirected",a=e._edges.values();return{[Symbol.iterator](){return this},next(){let i,o;for(;;){if(i=a.next(),i.done)return i;if(o=i.value,!(t&&o.undirected!==r))break}return{value:{edge:o.key,attributes:o.attributes,source:o.source.key,target:o.target.key,sourceAttributes:o.source.attributes,targetAttributes:o.target.attributes,undirected:o.undirected},done:!1}}}}function ca(e,n,t,r,a,i){let o=n?nc:tc,s;if(t!=="undirected"&&(r!=="out"&&(s=o(e,a.in,i),e&&s)||r!=="in"&&(s=o(e,a.out,i,r?void 0:a.key),e&&s))||t!=="directed"&&(s=o(e,a.undirected,i),e&&s))return s}function sc(e,n,t,r){let a=[];return ca(!1,e,n,t,r,function(i){a.push(i)}),a}function lc(e,n,t){let r=Yt();return e!=="undirected"&&(n!=="out"&&typeof t.in<"u"&&(r=ot(r,la(t.in))),n!=="in"&&typeof t.out<"u"&&(r=ot(r,la(t.out,n?void 0:t.key)))),e!=="directed"&&typeof t.undirected<"u"&&(r=ot(r,la(t.undirected))),r}function da(e,n,t,r,a,i,o){let s=t?ac:rc,l;if(n!=="undirected"&&(typeof a.in<"u"&&r!=="out"&&(l=s(e,a.in,i,o),e&&l)||typeof a.out<"u"&&r!=="in"&&(r||a.key!==i)&&(l=s(e,a.out,i,o),e&&l))||n!=="directed"&&typeof a.undirected<"u"&&(l=s(e,a.undirected,i,o),e&&l))return l}function uc(e,n,t,r,a){let i=[];return da(!1,e,n,t,r,a,function(o){i.push(o)}),i}function cc(e,n,t,r){let a=Yt();return e!=="undirected"&&(typeof t.in<"u"&&n!=="out"&&r in t.in&&(a=ot(a,ua(t.in,r))),typeof t.out<"u"&&n!=="in"&&r in t.out&&(n||t.key!==r)&&(a=ot(a,ua(t.out,r)))),e!=="directed"&&typeof t.undirected<"u"&&r in t.undirected&&(a=ot(a,ua(t.undirected,r))),a}function dc(e,n){let{name:t,type:r,direction:a}=n;e.prototype[t]=function(i,o){if(r!=="mixed"&&this.type!=="mixed"&&r!==this.type)return[];if(!arguments.length)return ic(this,r);if(arguments.length===1){i=""+i;let s=this._nodes.get(i);if(typeof s>"u")throw new H(`Graph.${t}: could not find the "${i}" node in the graph.`);return sc(this.multi,r==="mixed"?this.type:r,a,s)}if(arguments.length===2){i=""+i,o=""+o;let s=this._nodes.get(i);if(!s)throw new H(`Graph.${t}:  could not find the "${i}" source node in the graph.`);if(!this._nodes.has(o))throw new H(`Graph.${t}:  could not find the "${o}" target node in the graph.`);return uc(r,this.multi,a,s,o)}throw new V(`Graph.${t}: too many arguments (expecting 0, 1 or 2 and got ${arguments.length}).`)}}function hc(e,n){let{name:t,type:r,direction:a}=n,i="forEach"+t[0].toUpperCase()+t.slice(1,-1);e.prototype[i]=function(u,d,c){if(!(r!=="mixed"&&this.type!=="mixed"&&r!==this.type)){if(arguments.length===1)return c=u,ji(!1,this,r,c);if(arguments.length===2){u=""+u,c=d;let g=this._nodes.get(u);if(typeof g>"u")throw new H(`Graph.${i}: could not find the "${u}" node in the graph.`);return ca(!1,this.multi,r==="mixed"?this.type:r,a,g,c)}if(arguments.length===3){u=""+u,d=""+d;let g=this._nodes.get(u);if(!g)throw new H(`Graph.${i}:  could not find the "${u}" source node in the graph.`);if(!this._nodes.has(d))throw new H(`Graph.${i}:  could not find the "${d}" target node in the graph.`);return da(!1,r,this.multi,a,g,d,c)}throw new V(`Graph.${i}: too many arguments (expecting 1, 2 or 3 and got ${arguments.length}).`)}};let o="map"+t[0].toUpperCase()+t.slice(1);e.prototype[o]=function(){let u=Array.prototype.slice.call(arguments),d=u.pop(),c;if(u.length===0){let g=0;r!=="directed"&&(g+=this.undirectedSize),r!=="undirected"&&(g+=this.directedSize),c=new Array(g);let h=0;u.push((f,y,v,p,m,b,x)=>{c[h++]=d(f,y,v,p,m,b,x)})}else c=[],u.push((g,h,f,y,v,p,m)=>{c.push(d(g,h,f,y,v,p,m))});return this[i].apply(this,u),c};let s="filter"+t[0].toUpperCase()+t.slice(1);e.prototype[s]=function(){let u=Array.prototype.slice.call(arguments),d=u.pop(),c=[];return u.push((g,h,f,y,v,p,m)=>{d(g,h,f,y,v,p,m)&&c.push(g)}),this[i].apply(this,u),c};let l="reduce"+t[0].toUpperCase()+t.slice(1);e.prototype[l]=function(){let u=Array.prototype.slice.call(arguments);if(u.length<2||u.length>4)throw new V(`Graph.${l}: invalid number of arguments (expecting 2, 3 or 4 and got ${u.length}).`);if(typeof u[u.length-1]=="function"&&typeof u[u.length-2]!="function")throw new V(`Graph.${l}: missing initial value. You must provide it because the callback takes more than one argument and we cannot infer the initial value from the first iteration, as you could with a simple array.`);let d,c;u.length===2?(d=u[0],c=u[1],u=[]):u.length===3?(d=u[1],c=u[2],u=[u[0]]):u.length===4&&(d=u[2],c=u[3],u=[u[0],u[1]]);let g=c;return u.push((h,f,y,v,p,m,b)=>{g=d(g,h,f,y,v,p,m,b)}),this[i].apply(this,u),g}}function fc(e,n){let{name:t,type:r,direction:a}=n,i="find"+t[0].toUpperCase()+t.slice(1,-1);e.prototype[i]=function(l,u,d){if(r!=="mixed"&&this.type!=="mixed"&&r!==this.type)return!1;if(arguments.length===1)return d=l,ji(!0,this,r,d);if(arguments.length===2){l=""+l,d=u;let c=this._nodes.get(l);if(typeof c>"u")throw new H(`Graph.${i}: could not find the "${l}" node in the graph.`);return ca(!0,this.multi,r==="mixed"?this.type:r,a,c,d)}if(arguments.length===3){l=""+l,u=""+u;let c=this._nodes.get(l);if(!c)throw new H(`Graph.${i}:  could not find the "${l}" source node in the graph.`);if(!this._nodes.has(u))throw new H(`Graph.${i}:  could not find the "${u}" target node in the graph.`);return da(!0,r,this.multi,a,c,u,d)}throw new V(`Graph.${i}: too many arguments (expecting 1, 2 or 3 and got ${arguments.length}).`)};let o="some"+t[0].toUpperCase()+t.slice(1,-1);e.prototype[o]=function(){let l=Array.prototype.slice.call(arguments),u=l.pop();return l.push((c,g,h,f,y,v,p)=>u(c,g,h,f,y,v,p)),!!this[i].apply(this,l)};let s="every"+t[0].toUpperCase()+t.slice(1,-1);e.prototype[s]=function(){let l=Array.prototype.slice.call(arguments),u=l.pop();return l.push((c,g,h,f,y,v,p)=>!u(c,g,h,f,y,v,p)),!this[i].apply(this,l)}}function gc(e,n){let{name:t,type:r,direction:a}=n,i=t.slice(0,-1)+"Entries";e.prototype[i]=function(o,s){if(r!=="mixed"&&this.type!=="mixed"&&r!==this.type)return Yt();if(!arguments.length)return oc(this,r);if(arguments.length===1){o=""+o;let l=this._nodes.get(o);if(!l)throw new H(`Graph.${i}: could not find the "${o}" node in the graph.`);return lc(r,a,l)}if(arguments.length===2){o=""+o,s=""+s;let l=this._nodes.get(o);if(!l)throw new H(`Graph.${i}:  could not find the "${o}" source node in the graph.`);if(!this._nodes.has(s))throw new H(`Graph.${i}:  could not find the "${s}" target node in the graph.`);return cc(r,a,l,s)}throw new V(`Graph.${i}: too many arguments (expecting 0, 1 or 2 and got ${arguments.length}).`)}}function pc(e){ec.forEach(n=>{dc(e,n),hc(e,n),fc(e,n),gc(e,n)})}var vc=[{name:"neighbors",type:"mixed"},{name:"inNeighbors",type:"directed",direction:"in"},{name:"outNeighbors",type:"directed",direction:"out"},{name:"inboundNeighbors",type:"mixed",direction:"in"},{name:"outboundNeighbors",type:"mixed",direction:"out"},{name:"directedNeighbors",type:"directed"},{name:"undirectedNeighbors",type:"undirected"}];function fr(){this.A=null,this.B=null}fr.prototype.wrap=function(e){this.A===null?this.A=e:this.B===null&&(this.B=e)};fr.prototype.has=function(e){return this.A!==null&&e in this.A||this.B!==null&&e in this.B};function xn(e,n,t,r,a){for(let i in r){let o=r[i],s=o.source,l=o.target,u=s===t?l:s;if(n&&n.has(u.key))continue;let d=a(u.key,u.attributes);if(e&&d)return u.key}}function ha(e,n,t,r,a){if(n!=="mixed"){if(n==="undirected")return xn(e,null,r,r.undirected,a);if(typeof t=="string")return xn(e,null,r,r[t],a)}let i=new fr,o;if(n!=="undirected"){if(t!=="out"){if(o=xn(e,null,r,r.in,a),e&&o)return o;i.wrap(r.in)}if(t!=="in"){if(o=xn(e,i,r,r.out,a),e&&o)return o;i.wrap(r.out)}}if(n!=="directed"&&(o=xn(e,i,r,r.undirected,a),e&&o))return o}function mc(e,n,t){if(e!=="mixed"){if(e==="undirected")return Object.keys(t.undirected);if(typeof n=="string")return Object.keys(t[n])}let r=[];return ha(!1,e,n,t,function(a){r.push(a)}),r}function Tn(e,n,t){let r=Object.keys(t),a=r.length,i=0;return{[Symbol.iterator](){return this},next(){let o=null;do{if(i>=a)return e&&e.wrap(t),{done:!0};let s=t[r[i++]],l=s.source,u=s.target;if(o=l===n?u:l,e&&e.has(o.key)){o=null;continue}}while(o===null);return{done:!1,value:{neighbor:o.key,attributes:o.attributes}}}}}function bc(e,n,t){if(e!=="mixed"){if(e==="undirected")return Tn(null,t,t.undirected);if(typeof n=="string")return Tn(null,t,t[n])}let r=Yt(),a=new fr;return e!=="undirected"&&(n!=="out"&&(r=ot(r,Tn(a,t,t.in))),n!=="in"&&(r=ot(r,Tn(a,t,t.out)))),e!=="directed"&&(r=ot(r,Tn(a,t,t.undirected))),r}function yc(e,n){let{name:t,type:r,direction:a}=n;e.prototype[t]=function(i){if(r!=="mixed"&&this.type!=="mixed"&&r!==this.type)return[];i=""+i;let o=this._nodes.get(i);if(typeof o>"u")throw new H(`Graph.${t}: could not find the "${i}" node in the graph.`);return mc(r==="mixed"?this.type:r,a,o)}}function _c(e,n){let{name:t,type:r,direction:a}=n,i="forEach"+t[0].toUpperCase()+t.slice(1,-1);e.prototype[i]=function(u,d){if(r!=="mixed"&&this.type!=="mixed"&&r!==this.type)return;u=""+u;let c=this._nodes.get(u);if(typeof c>"u")throw new H(`Graph.${i}: could not find the "${u}" node in the graph.`);ha(!1,r==="mixed"?this.type:r,a,c,d)};let o="map"+t[0].toUpperCase()+t.slice(1);e.prototype[o]=function(u,d){let c=[];return this[i](u,(g,h)=>{c.push(d(g,h))}),c};let s="filter"+t[0].toUpperCase()+t.slice(1);e.prototype[s]=function(u,d){let c=[];return this[i](u,(g,h)=>{d(g,h)&&c.push(g)}),c};let l="reduce"+t[0].toUpperCase()+t.slice(1);e.prototype[l]=function(u,d,c){if(arguments.length<3)throw new V(`Graph.${l}: missing initial value. You must provide it because the callback takes more than one argument and we cannot infer the initial value from the first iteration, as you could with a simple array.`);let g=c;return this[i](u,(h,f)=>{g=d(g,h,f)}),g}}function xc(e,n){let{name:t,type:r,direction:a}=n,i=t[0].toUpperCase()+t.slice(1,-1),o="find"+i;e.prototype[o]=function(u,d){if(r!=="mixed"&&this.type!=="mixed"&&r!==this.type)return;u=""+u;let c=this._nodes.get(u);if(typeof c>"u")throw new H(`Graph.${o}: could not find the "${u}" node in the graph.`);return ha(!0,r==="mixed"?this.type:r,a,c,d)};let s="some"+i;e.prototype[s]=function(u,d){return!!this[o](u,d)};let l="every"+i;e.prototype[l]=function(u,d){return!this[o](u,(g,h)=>!d(g,h))}}function Tc(e,n){let{name:t,type:r,direction:a}=n,i=t.slice(0,-1)+"Entries";e.prototype[i]=function(o){if(r!=="mixed"&&this.type!=="mixed"&&r!==this.type)return Yt();o=""+o;let s=this._nodes.get(o);if(typeof s>"u")throw new H(`Graph.${i}: could not find the "${o}" node in the graph.`);return bc(r==="mixed"?this.type:r,a,s)}}function Ec(e){vc.forEach(n=>{yc(e,n),_c(e,n),xc(e,n),Tc(e,n)})}function sr(e,n,t,r,a){let i=r._nodes.values(),o=r.type,s,l,u,d,c,g,h;for(;s=i.next(),s.done!==!0;){let f=!1;if(l=s.value,o!=="undirected"){d=l.out;for(u in d){c=d[u];do{if(g=c.target,f=!0,h=a(l.key,g.key,l.attributes,g.attributes,c.key,c.attributes,c.undirected),e&&h)return c;c=c.next}while(c)}}if(o!=="directed"){d=l.undirected;for(u in d)if(!(n&&l.key>u)){c=d[u];do{if(g=c.target,g.key!==u&&(g=c.source),f=!0,h=a(l.key,g.key,l.attributes,g.attributes,c.key,c.attributes,c.undirected),e&&h)return c;c=c.next}while(c)}}if(t&&!f&&(h=a(l.key,null,l.attributes,null,null,null,null),e&&h))return null}}function Sc(e,n){let t={key:e};return Bi(n.attributes)||(t.attributes=Te({},n.attributes)),t}function Rc(e,n,t){let r={key:n,source:t.source.key,target:t.target.key};return Bi(t.attributes)||(r.attributes=Te({},t.attributes)),e==="mixed"&&t.undirected&&(r.undirected=!0),r}function Ac(e){if(!Ae(e))throw new V('Graph.import: invalid serialized node. A serialized node should be a plain object with at least a "key" property.');if(!("key"in e))throw new V("Graph.import: serialized node is missing its key.");if("attributes"in e&&(!Ae(e.attributes)||e.attributes===null))throw new V("Graph.import: invalid attributes. Attributes should be a plain object, null or omitted.")}function Dc(e){if(!Ae(e))throw new V('Graph.import: invalid serialized edge. A serialized edge should be a plain object with at least a "source" & "target" property.');if(!("source"in e))throw new V("Graph.import: serialized edge is missing its source.");if(!("target"in e))throw new V("Graph.import: serialized edge is missing its target.");if("attributes"in e&&(!Ae(e.attributes)||e.attributes===null))throw new V("Graph.import: invalid attributes. Attributes should be a plain object, null or omitted.");if("undirected"in e&&typeof e.undirected!="boolean")throw new V("Graph.import: invalid undirectedness information. Undirected should be boolean or omitted.")}var wc=Cu(),kc=new Set(["directed","undirected","mixed"]),Ii=new Set(["domain","_events","_eventsCount","_maxListeners"]),Cc=[{name:e=>`${e}Edge`,generateKey:!0},{name:e=>`${e}DirectedEdge`,generateKey:!0,type:"directed"},{name:e=>`${e}UndirectedEdge`,generateKey:!0,type:"undirected"},{name:e=>`${e}EdgeWithKey`},{name:e=>`${e}DirectedEdgeWithKey`,type:"directed"},{name:e=>`${e}UndirectedEdgeWithKey`,type:"undirected"}],Fc={allowSelfLoops:!0,multi:!1,type:"mixed"};function Lc(e,n,t){if(t&&!Ae(t))throw new V(`Graph.addNode: invalid attributes. Expecting an object but got "${t}"`);if(n=""+n,t=t||{},e._nodes.has(n))throw new Z(`Graph.addNode: the "${n}" node already exist in the graph.`);let r=new e.NodeDataClass(n,t);return e._nodes.set(n,r),e.emit("nodeAdded",{key:n,attributes:t}),r}function Gi(e,n,t){let r=new e.NodeDataClass(n,t);return e._nodes.set(n,r),e.emit("nodeAdded",{key:n,attributes:t}),r}function $i(e,n,t,r,a,i,o,s){if(!r&&e.type==="undirected")throw new Z(`Graph.${n}: you cannot add a directed edge to an undirected graph. Use the #.addEdge or #.addUndirectedEdge instead.`);if(r&&e.type==="directed")throw new Z(`Graph.${n}: you cannot add an undirected edge to a directed graph. Use the #.addEdge or #.addDirectedEdge instead.`);if(s&&!Ae(s))throw new V(`Graph.${n}: invalid attributes. Expecting an object but got "${s}"`);if(i=""+i,o=""+o,s=s||{},!e.allowSelfLoops&&i===o)throw new Z(`Graph.${n}: source & target are the same ("${i}"), thus creating a loop explicitly forbidden by this graph 'allowSelfLoops' option set to false.`);let l=e._nodes.get(i),u=e._nodes.get(o);if(!l)throw new H(`Graph.${n}: source node "${i}" not found.`);if(!u)throw new H(`Graph.${n}: target node "${o}" not found.`);let d={key:null,undirected:r,source:i,target:o,attributes:s};if(t)a=e._edgeKeyGenerator();else if(a=""+a,e._edges.has(a))throw new Z(`Graph.${n}: the "${a}" edge already exists in the graph.`);if(!e.multi&&(r?typeof l.undirected[o]<"u":typeof l.out[o]<"u"))throw new Z(`Graph.${n}: an edge linking "${i}" to "${o}" already exists. If you really want to add multiple edges linking those nodes, you should create a multi graph by using the 'multi' option.`);let c=new Jt(r,a,l,u,s);e._edges.set(a,c);let g=i===o;return r?(l.undirectedDegree++,u.undirectedDegree++,g&&(l.undirectedLoops++,e._undirectedSelfLoopCount++)):(l.outDegree++,u.inDegree++,g&&(l.directedLoops++,e._directedSelfLoopCount++)),e.multi?c.attachMulti():c.attach(),r?e._undirectedSize++:e._directedSize++,d.key=a,e.emit("edgeAdded",d),a}function Pc(e,n,t,r,a,i,o,s,l){if(!r&&e.type==="undirected")throw new Z(`Graph.${n}: you cannot merge/update a directed edge to an undirected graph. Use the #.mergeEdge/#.updateEdge or #.addUndirectedEdge instead.`);if(r&&e.type==="directed")throw new Z(`Graph.${n}: you cannot merge/update an undirected edge to a directed graph. Use the #.mergeEdge/#.updateEdge or #.addDirectedEdge instead.`);if(s){if(l){if(typeof s!="function")throw new V(`Graph.${n}: invalid updater function. Expecting a function but got "${s}"`)}else if(!Ae(s))throw new V(`Graph.${n}: invalid attributes. Expecting an object but got "${s}"`)}i=""+i,o=""+o;let u;if(l&&(u=s,s=void 0),!e.allowSelfLoops&&i===o)throw new Z(`Graph.${n}: source & target are the same ("${i}"), thus creating a loop explicitly forbidden by this graph 'allowSelfLoops' option set to false.`);let d=e._nodes.get(i),c=e._nodes.get(o),g,h;if(!t&&(g=e._edges.get(a),g)){if((g.source.key!==i||g.target.key!==o)&&(!r||g.source.key!==o||g.target.key!==i))throw new Z(`Graph.${n}: inconsistency detected when attempting to merge the "${a}" edge with "${i}" source & "${o}" target vs. ("${g.source.key}", "${g.target.key}").`);h=g}if(!h&&!e.multi&&d&&(h=r?d.undirected[o]:d.out[o]),h){let m=[h.key,!1,!1,!1];if(l?!u:!s)return m;if(l){let b=h.attributes;h.attributes=u(b),e.emit("edgeAttributesUpdated",{type:"replace",key:h.key,attributes:h.attributes})}else Te(h.attributes,s),e.emit("edgeAttributesUpdated",{type:"merge",key:h.key,attributes:h.attributes,data:s});return m}s=s||{},l&&u&&(s=u(s));let f={key:null,undirected:r,source:i,target:o,attributes:s};if(t)a=e._edgeKeyGenerator();else if(a=""+a,e._edges.has(a))throw new Z(`Graph.${n}: the "${a}" edge already exists in the graph.`);let y=!1,v=!1;d||(d=Gi(e,i,{}),y=!0,i===o&&(c=d,v=!0)),c||(c=Gi(e,o,{}),v=!0),g=new Jt(r,a,d,c,s),e._edges.set(a,g);let p=i===o;return r?(d.undirectedDegree++,c.undirectedDegree++,p&&(d.undirectedLoops++,e._undirectedSelfLoopCount++)):(d.outDegree++,c.inDegree++,p&&(d.directedLoops++,e._directedSelfLoopCount++)),e.multi?g.attachMulti():g.attach(),r?e._undirectedSize++:e._directedSize++,f.key=a,e.emit("edgeAdded",f),[a,!0,y,v]}function Xt(e,n){e._edges.delete(n.key);let{source:t,target:r,attributes:a}=n,i=n.undirected,o=t===r;i?(t.undirectedDegree--,r.undirectedDegree--,o&&(t.undirectedLoops--,e._undirectedSelfLoopCount--)):(t.outDegree--,r.inDegree--,o&&(t.directedLoops--,e._directedSelfLoopCount--)),e.multi?n.detachMulti():n.detach(),i?e._undirectedSize--:e._directedSize--,e.emit("edgeDropped",{key:n.key,attributes:a,source:t.key,target:r.key,undirected:i})}var de=class e extends zi.EventEmitter{constructor(n){if(super(),n=Te({},Fc,n),typeof n.multi!="boolean")throw new V(`Graph.constructor: invalid 'multi' option. Expecting a boolean but got "${n.multi}".`);if(!kc.has(n.type))throw new V(`Graph.constructor: invalid 'type' option. Should be one of "mixed", "directed" or "undirected" but got "${n.type}".`);if(typeof n.allowSelfLoops!="boolean")throw new V(`Graph.constructor: invalid 'allowSelfLoops' option. Expecting a boolean but got "${n.allowSelfLoops}".`);let t=n.type==="mixed"?Ni:n.type==="directed"?Mi:Wi;Be(this,"NodeDataClass",t);let r="geid_"+wc()+"_",a=0,i=()=>{let o;do o=r+a++;while(this._edges.has(o));return o};Be(this,"_attributes",{}),Be(this,"_nodes",new Map),Be(this,"_edges",new Map),Be(this,"_directedSize",0),Be(this,"_undirectedSize",0),Be(this,"_directedSelfLoopCount",0),Be(this,"_undirectedSelfLoopCount",0),Be(this,"_edgeKeyGenerator",i),Be(this,"_options",n),Ii.forEach(o=>Be(this,o,this[o])),qe(this,"order",()=>this._nodes.size),qe(this,"size",()=>this._edges.size),qe(this,"directedSize",()=>this._directedSize),qe(this,"undirectedSize",()=>this._undirectedSize),qe(this,"selfLoopCount",()=>this._directedSelfLoopCount+this._undirectedSelfLoopCount),qe(this,"directedSelfLoopCount",()=>this._directedSelfLoopCount),qe(this,"undirectedSelfLoopCount",()=>this._undirectedSelfLoopCount),qe(this,"multi",this._options.multi),qe(this,"type",this._options.type),qe(this,"allowSelfLoops",this._options.allowSelfLoops),qe(this,"implementation",()=>"graphology")}_resetInstanceCounters(){this._directedSize=0,this._undirectedSize=0,this._directedSelfLoopCount=0,this._undirectedSelfLoopCount=0}hasNode(n){return this._nodes.has(""+n)}hasDirectedEdge(n,t){if(this.type==="undirected")return!1;if(arguments.length===1){let r=""+n,a=this._edges.get(r);return!!a&&!a.undirected}else if(arguments.length===2){n=""+n,t=""+t;let r=this._nodes.get(n);return r?r.out.hasOwnProperty(t):!1}throw new V(`Graph.hasDirectedEdge: invalid arity (${arguments.length}, instead of 1 or 2). You can either ask for an edge id or for the existence of an edge between a source & a target.`)}hasUndirectedEdge(n,t){if(this.type==="directed")return!1;if(arguments.length===1){let r=""+n,a=this._edges.get(r);return!!a&&a.undirected}else if(arguments.length===2){n=""+n,t=""+t;let r=this._nodes.get(n);return r?r.undirected.hasOwnProperty(t):!1}throw new V(`Graph.hasDirectedEdge: invalid arity (${arguments.length}, instead of 1 or 2). You can either ask for an edge id or for the existence of an edge between a source & a target.`)}hasEdge(n,t){if(arguments.length===1){let r=""+n;return this._edges.has(r)}else if(arguments.length===2){n=""+n,t=""+t;let r=this._nodes.get(n);return r?typeof r.out<"u"&&r.out.hasOwnProperty(t)||typeof r.undirected<"u"&&r.undirected.hasOwnProperty(t):!1}throw new V(`Graph.hasEdge: invalid arity (${arguments.length}, instead of 1 or 2). You can either ask for an edge id or for the existence of an edge between a source & a target.`)}directedEdge(n,t){if(this.type==="undirected")return;if(n=""+n,t=""+t,this.multi)throw new Z("Graph.directedEdge: this method is irrelevant with multigraphs since there might be multiple edges between source & target. See #.directedEdges instead.");let r=this._nodes.get(n);if(!r)throw new H(`Graph.directedEdge: could not find the "${n}" source node in the graph.`);if(!this._nodes.has(t))throw new H(`Graph.directedEdge: could not find the "${t}" target node in the graph.`);let a=r.out&&r.out[t]||void 0;if(a)return a.key}undirectedEdge(n,t){if(this.type==="directed")return;if(n=""+n,t=""+t,this.multi)throw new Z("Graph.undirectedEdge: this method is irrelevant with multigraphs since there might be multiple edges between source & target. See #.undirectedEdges instead.");let r=this._nodes.get(n);if(!r)throw new H(`Graph.undirectedEdge: could not find the "${n}" source node in the graph.`);if(!this._nodes.has(t))throw new H(`Graph.undirectedEdge: could not find the "${t}" target node in the graph.`);let a=r.undirected&&r.undirected[t]||void 0;if(a)return a.key}edge(n,t){if(this.multi)throw new Z("Graph.edge: this method is irrelevant with multigraphs since there might be multiple edges between source & target. See #.edges instead.");n=""+n,t=""+t;let r=this._nodes.get(n);if(!r)throw new H(`Graph.edge: could not find the "${n}" source node in the graph.`);if(!this._nodes.has(t))throw new H(`Graph.edge: could not find the "${t}" target node in the graph.`);let a=r.out&&r.out[t]||r.undirected&&r.undirected[t]||void 0;if(a)return a.key}areDirectedNeighbors(n,t){n=""+n,t=""+t;let r=this._nodes.get(n);if(!r)throw new H(`Graph.areDirectedNeighbors: could not find the "${n}" node in the graph.`);return this.type==="undirected"?!1:t in r.in||t in r.out}areOutNeighbors(n,t){n=""+n,t=""+t;let r=this._nodes.get(n);if(!r)throw new H(`Graph.areOutNeighbors: could not find the "${n}" node in the graph.`);return this.type==="undirected"?!1:t in r.out}areInNeighbors(n,t){n=""+n,t=""+t;let r=this._nodes.get(n);if(!r)throw new H(`Graph.areInNeighbors: could not find the "${n}" node in the graph.`);return this.type==="undirected"?!1:t in r.in}areUndirectedNeighbors(n,t){n=""+n,t=""+t;let r=this._nodes.get(n);if(!r)throw new H(`Graph.areUndirectedNeighbors: could not find the "${n}" node in the graph.`);return this.type==="directed"?!1:t in r.undirected}areNeighbors(n,t){n=""+n,t=""+t;let r=this._nodes.get(n);if(!r)throw new H(`Graph.areNeighbors: could not find the "${n}" node in the graph.`);return this.type!=="undirected"&&(t in r.in||t in r.out)||this.type!=="directed"&&t in r.undirected}areInboundNeighbors(n,t){n=""+n,t=""+t;let r=this._nodes.get(n);if(!r)throw new H(`Graph.areInboundNeighbors: could not find the "${n}" node in the graph.`);return this.type!=="undirected"&&t in r.in||this.type!=="directed"&&t in r.undirected}areOutboundNeighbors(n,t){n=""+n,t=""+t;let r=this._nodes.get(n);if(!r)throw new H(`Graph.areOutboundNeighbors: could not find the "${n}" node in the graph.`);return this.type!=="undirected"&&t in r.out||this.type!=="directed"&&t in r.undirected}inDegree(n){n=""+n;let t=this._nodes.get(n);if(!t)throw new H(`Graph.inDegree: could not find the "${n}" node in the graph.`);return this.type==="undirected"?0:t.inDegree}outDegree(n){n=""+n;let t=this._nodes.get(n);if(!t)throw new H(`Graph.outDegree: could not find the "${n}" node in the graph.`);return this.type==="undirected"?0:t.outDegree}directedDegree(n){n=""+n;let t=this._nodes.get(n);if(!t)throw new H(`Graph.directedDegree: could not find the "${n}" node in the graph.`);return this.type==="undirected"?0:t.inDegree+t.outDegree}undirectedDegree(n){n=""+n;let t=this._nodes.get(n);if(!t)throw new H(`Graph.undirectedDegree: could not find the "${n}" node in the graph.`);return this.type==="directed"?0:t.undirectedDegree}inboundDegree(n){n=""+n;let t=this._nodes.get(n);if(!t)throw new H(`Graph.inboundDegree: could not find the "${n}" node in the graph.`);let r=0;return this.type!=="directed"&&(r+=t.undirectedDegree),this.type!=="undirected"&&(r+=t.inDegree),r}outboundDegree(n){n=""+n;let t=this._nodes.get(n);if(!t)throw new H(`Graph.outboundDegree: could not find the "${n}" node in the graph.`);let r=0;return this.type!=="directed"&&(r+=t.undirectedDegree),this.type!=="undirected"&&(r+=t.outDegree),r}degree(n){n=""+n;let t=this._nodes.get(n);if(!t)throw new H(`Graph.degree: could not find the "${n}" node in the graph.`);let r=0;return this.type!=="directed"&&(r+=t.undirectedDegree),this.type!=="undirected"&&(r+=t.inDegree+t.outDegree),r}inDegreeWithoutSelfLoops(n){n=""+n;let t=this._nodes.get(n);if(!t)throw new H(`Graph.inDegreeWithoutSelfLoops: could not find the "${n}" node in the graph.`);return this.type==="undirected"?0:t.inDegree-t.directedLoops}outDegreeWithoutSelfLoops(n){n=""+n;let t=this._nodes.get(n);if(!t)throw new H(`Graph.outDegreeWithoutSelfLoops: could not find the "${n}" node in the graph.`);return this.type==="undirected"?0:t.outDegree-t.directedLoops}directedDegreeWithoutSelfLoops(n){n=""+n;let t=this._nodes.get(n);if(!t)throw new H(`Graph.directedDegreeWithoutSelfLoops: could not find the "${n}" node in the graph.`);return this.type==="undirected"?0:t.inDegree+t.outDegree-t.directedLoops*2}undirectedDegreeWithoutSelfLoops(n){n=""+n;let t=this._nodes.get(n);if(!t)throw new H(`Graph.undirectedDegreeWithoutSelfLoops: could not find the "${n}" node in the graph.`);return this.type==="directed"?0:t.undirectedDegree-t.undirectedLoops*2}inboundDegreeWithoutSelfLoops(n){n=""+n;let t=this._nodes.get(n);if(!t)throw new H(`Graph.inboundDegreeWithoutSelfLoops: could not find the "${n}" node in the graph.`);let r=0,a=0;return this.type!=="directed"&&(r+=t.undirectedDegree,a+=t.undirectedLoops*2),this.type!=="undirected"&&(r+=t.inDegree,a+=t.directedLoops),r-a}outboundDegreeWithoutSelfLoops(n){n=""+n;let t=this._nodes.get(n);if(!t)throw new H(`Graph.outboundDegreeWithoutSelfLoops: could not find the "${n}" node in the graph.`);let r=0,a=0;return this.type!=="directed"&&(r+=t.undirectedDegree,a+=t.undirectedLoops*2),this.type!=="undirected"&&(r+=t.outDegree,a+=t.directedLoops),r-a}degreeWithoutSelfLoops(n){n=""+n;let t=this._nodes.get(n);if(!t)throw new H(`Graph.degreeWithoutSelfLoops: could not find the "${n}" node in the graph.`);let r=0,a=0;return this.type!=="directed"&&(r+=t.undirectedDegree,a+=t.undirectedLoops*2),this.type!=="undirected"&&(r+=t.inDegree+t.outDegree,a+=t.directedLoops*2),r-a}source(n){n=""+n;let t=this._edges.get(n);if(!t)throw new H(`Graph.source: could not find the "${n}" edge in the graph.`);return t.source.key}target(n){n=""+n;let t=this._edges.get(n);if(!t)throw new H(`Graph.target: could not find the "${n}" edge in the graph.`);return t.target.key}extremities(n){n=""+n;let t=this._edges.get(n);if(!t)throw new H(`Graph.extremities: could not find the "${n}" edge in the graph.`);return[t.source.key,t.target.key]}opposite(n,t){n=""+n,t=""+t;let r=this._edges.get(t);if(!r)throw new H(`Graph.opposite: could not find the "${t}" edge in the graph.`);let a=r.source.key,i=r.target.key;if(n===a)return i;if(n===i)return a;throw new H(`Graph.opposite: the "${n}" node is not attached to the "${t}" edge (${a}, ${i}).`)}hasExtremity(n,t){n=""+n,t=""+t;let r=this._edges.get(n);if(!r)throw new H(`Graph.hasExtremity: could not find the "${n}" edge in the graph.`);return r.source.key===t||r.target.key===t}isUndirected(n){n=""+n;let t=this._edges.get(n);if(!t)throw new H(`Graph.isUndirected: could not find the "${n}" edge in the graph.`);return t.undirected}isDirected(n){n=""+n;let t=this._edges.get(n);if(!t)throw new H(`Graph.isDirected: could not find the "${n}" edge in the graph.`);return!t.undirected}isSelfLoop(n){n=""+n;let t=this._edges.get(n);if(!t)throw new H(`Graph.isSelfLoop: could not find the "${n}" edge in the graph.`);return t.source===t.target}addNode(n,t){return Lc(this,n,t).key}mergeNode(n,t){if(t&&!Ae(t))throw new V(`Graph.mergeNode: invalid attributes. Expecting an object but got "${t}"`);n=""+n,t=t||{};let r=this._nodes.get(n);return r?(t&&(Te(r.attributes,t),this.emit("nodeAttributesUpdated",{type:"merge",key:n,attributes:r.attributes,data:t})),[n,!1]):(r=new this.NodeDataClass(n,t),this._nodes.set(n,r),this.emit("nodeAdded",{key:n,attributes:t}),[n,!0])}updateNode(n,t){if(t&&typeof t!="function")throw new V(`Graph.updateNode: invalid updater function. Expecting a function but got "${t}"`);n=""+n;let r=this._nodes.get(n);if(r){if(t){let i=r.attributes;r.attributes=t(i),this.emit("nodeAttributesUpdated",{type:"replace",key:n,attributes:r.attributes})}return[n,!1]}let a=t?t({}):{};return r=new this.NodeDataClass(n,a),this._nodes.set(n,r),this.emit("nodeAdded",{key:n,attributes:a}),[n,!0]}dropNode(n){n=""+n;let t=this._nodes.get(n);if(!t)throw new H(`Graph.dropNode: could not find the "${n}" node in the graph.`);let r;if(this.type!=="undirected"){for(let a in t.out){r=t.out[a];do Xt(this,r),r=r.next;while(r)}for(let a in t.in){r=t.in[a];do Xt(this,r),r=r.next;while(r)}}if(this.type!=="directed")for(let a in t.undirected){r=t.undirected[a];do Xt(this,r),r=r.next;while(r)}this._nodes.delete(n),this.emit("nodeDropped",{key:n,attributes:t.attributes})}dropEdge(n){let t;if(arguments.length>1){let r=""+arguments[0],a=""+arguments[1];if(t=Ne(this,r,a,this.type),!t)throw new H(`Graph.dropEdge: could not find the "${r}" -> "${a}" edge in the graph.`)}else if(n=""+n,t=this._edges.get(n),!t)throw new H(`Graph.dropEdge: could not find the "${n}" edge in the graph.`);return Xt(this,t),this}dropDirectedEdge(n,t){if(arguments.length<2)throw new Z("Graph.dropDirectedEdge: it does not make sense to try and drop a directed edge by key. What if the edge with this key is undirected? Use #.dropEdge for this purpose instead.");if(this.multi)throw new Z("Graph.dropDirectedEdge: cannot use a {source,target} combo when dropping an edge in a MultiGraph since we cannot infer the one you want to delete as there could be multiple ones.");n=""+n,t=""+t;let r=Ne(this,n,t,"directed");if(!r)throw new H(`Graph.dropDirectedEdge: could not find a "${n}" -> "${t}" edge in the graph.`);return Xt(this,r),this}dropUndirectedEdge(n,t){if(arguments.length<2)throw new Z("Graph.dropUndirectedEdge: it does not make sense to drop a directed edge by key. What if the edge with this key is undirected? Use #.dropEdge for this purpose instead.");if(this.multi)throw new Z("Graph.dropUndirectedEdge: cannot use a {source,target} combo when dropping an edge in a MultiGraph since we cannot infer the one you want to delete as there could be multiple ones.");let r=Ne(this,n,t,"undirected");if(!r)throw new H(`Graph.dropUndirectedEdge: could not find a "${n}" -> "${t}" edge in the graph.`);return Xt(this,r),this}clear(){this._edges.clear(),this._nodes.clear(),this._resetInstanceCounters(),this.emit("cleared")}clearEdges(){let n=this._nodes.values(),t;for(;t=n.next(),t.done!==!0;)t.value.clear();this._edges.clear(),this._resetInstanceCounters(),this.emit("edgesCleared")}getAttribute(n){return this._attributes[n]}getAttributes(){return this._attributes}hasAttribute(n){return this._attributes.hasOwnProperty(n)}setAttribute(n,t){return this._attributes[n]=t,this.emit("attributesUpdated",{type:"set",attributes:this._attributes,name:n}),this}updateAttribute(n,t){if(typeof t!="function")throw new V("Graph.updateAttribute: updater should be a function.");let r=this._attributes[n];return this._attributes[n]=t(r),this.emit("attributesUpdated",{type:"set",attributes:this._attributes,name:n}),this}removeAttribute(n){return delete this._attributes[n],this.emit("attributesUpdated",{type:"remove",attributes:this._attributes,name:n}),this}replaceAttributes(n){if(!Ae(n))throw new V("Graph.replaceAttributes: provided attributes are not a plain object.");return this._attributes=n,this.emit("attributesUpdated",{type:"replace",attributes:this._attributes}),this}mergeAttributes(n){if(!Ae(n))throw new V("Graph.mergeAttributes: provided attributes are not a plain object.");return Te(this._attributes,n),this.emit("attributesUpdated",{type:"merge",attributes:this._attributes,data:n}),this}updateAttributes(n){if(typeof n!="function")throw new V("Graph.updateAttributes: provided updater is not a function.");return this._attributes=n(this._attributes),this.emit("attributesUpdated",{type:"update",attributes:this._attributes}),this}updateEachNodeAttributes(n,t){if(typeof n!="function")throw new V("Graph.updateEachNodeAttributes: expecting an updater function.");if(t&&!Pi(t))throw new V("Graph.updateEachNodeAttributes: invalid hints. Expecting an object having the following shape: {attributes?: [string]}");let r=this._nodes.values(),a,i;for(;a=r.next(),a.done!==!0;)i=a.value,i.attributes=n(i.key,i.attributes);this.emit("eachNodeAttributesUpdated",{hints:t||null})}updateEachEdgeAttributes(n,t){if(typeof n!="function")throw new V("Graph.updateEachEdgeAttributes: expecting an updater function.");if(t&&!Pi(t))throw new V("Graph.updateEachEdgeAttributes: invalid hints. Expecting an object having the following shape: {attributes?: [string]}");let r=this._edges.values(),a,i,o,s;for(;a=r.next(),a.done!==!0;)i=a.value,o=i.source,s=i.target,i.attributes=n(i.key,i.attributes,o.key,s.key,o.attributes,s.attributes,i.undirected);this.emit("eachEdgeAttributesUpdated",{hints:t||null})}forEachAdjacencyEntry(n){if(typeof n!="function")throw new V("Graph.forEachAdjacencyEntry: expecting a callback.");sr(!1,!1,!1,this,n)}forEachAdjacencyEntryWithOrphans(n){if(typeof n!="function")throw new V("Graph.forEachAdjacencyEntryWithOrphans: expecting a callback.");sr(!1,!1,!0,this,n)}forEachAssymetricAdjacencyEntry(n){if(typeof n!="function")throw new V("Graph.forEachAssymetricAdjacencyEntry: expecting a callback.");sr(!1,!0,!1,this,n)}forEachAssymetricAdjacencyEntryWithOrphans(n){if(typeof n!="function")throw new V("Graph.forEachAssymetricAdjacencyEntryWithOrphans: expecting a callback.");sr(!1,!0,!0,this,n)}nodes(){return Array.from(this._nodes.keys())}forEachNode(n){if(typeof n!="function")throw new V("Graph.forEachNode: expecting a callback.");let t=this._nodes.values(),r,a;for(;r=t.next(),r.done!==!0;)a=r.value,n(a.key,a.attributes)}findNode(n){if(typeof n!="function")throw new V("Graph.findNode: expecting a callback.");let t=this._nodes.values(),r,a;for(;r=t.next(),r.done!==!0;)if(a=r.value,n(a.key,a.attributes))return a.key}mapNodes(n){if(typeof n!="function")throw new V("Graph.mapNode: expecting a callback.");let t=this._nodes.values(),r,a,i=new Array(this.order),o=0;for(;r=t.next(),r.done!==!0;)a=r.value,i[o++]=n(a.key,a.attributes);return i}someNode(n){if(typeof n!="function")throw new V("Graph.someNode: expecting a callback.");let t=this._nodes.values(),r,a;for(;r=t.next(),r.done!==!0;)if(a=r.value,n(a.key,a.attributes))return!0;return!1}everyNode(n){if(typeof n!="function")throw new V("Graph.everyNode: expecting a callback.");let t=this._nodes.values(),r,a;for(;r=t.next(),r.done!==!0;)if(a=r.value,!n(a.key,a.attributes))return!1;return!0}filterNodes(n){if(typeof n!="function")throw new V("Graph.filterNodes: expecting a callback.");let t=this._nodes.values(),r,a,i=[];for(;r=t.next(),r.done!==!0;)a=r.value,n(a.key,a.attributes)&&i.push(a.key);return i}reduceNodes(n,t){if(typeof n!="function")throw new V("Graph.reduceNodes: expecting a callback.");if(arguments.length<2)throw new V("Graph.reduceNodes: missing initial value. You must provide it because the callback takes more than one argument and we cannot infer the initial value from the first iteration, as you could with a simple array.");let r=t,a=this._nodes.values(),i,o;for(;i=a.next(),i.done!==!0;)o=i.value,r=n(r,o.key,o.attributes);return r}nodeEntries(){let n=this._nodes.values();return{[Symbol.iterator](){return this},next(){let t=n.next();if(t.done)return t;let r=t.value;return{value:{node:r.key,attributes:r.attributes},done:!1}}}}export(){let n=new Array(this._nodes.size),t=0;this._nodes.forEach((a,i)=>{n[t++]=Sc(i,a)});let r=new Array(this._edges.size);return t=0,this._edges.forEach((a,i)=>{r[t++]=Rc(this.type,i,a)}),{options:{type:this.type,multi:this.multi,allowSelfLoops:this.allowSelfLoops},attributes:this.getAttributes(),nodes:n,edges:r}}import(n,t=!1){if(n instanceof e)return n.forEachNode((l,u)=>{t?this.mergeNode(l,u):this.addNode(l,u)}),n.forEachEdge((l,u,d,c,g,h,f)=>{t?f?this.mergeUndirectedEdgeWithKey(l,d,c,u):this.mergeDirectedEdgeWithKey(l,d,c,u):f?this.addUndirectedEdgeWithKey(l,d,c,u):this.addDirectedEdgeWithKey(l,d,c,u)}),this;if(!Ae(n))throw new V("Graph.import: invalid argument. Expecting a serialized graph or, alternatively, a Graph instance.");if(n.attributes){if(!Ae(n.attributes))throw new V("Graph.import: invalid attributes. Expecting a plain object.");t?this.mergeAttributes(n.attributes):this.replaceAttributes(n.attributes)}let r,a,i,o,s;if(n.nodes){if(i=n.nodes,!Array.isArray(i))throw new V("Graph.import: invalid nodes. Expecting an array.");for(r=0,a=i.length;r<a;r++){o=i[r],Ac(o);let{key:l,attributes:u}=o;t?this.mergeNode(l,u):this.addNode(l,u)}}if(n.edges){let l=!1;if(this.type==="undirected"&&(l=!0),i=n.edges,!Array.isArray(i))throw new V("Graph.import: invalid edges. Expecting an array.");for(r=0,a=i.length;r<a;r++){s=i[r],Dc(s);let{source:u,target:d,attributes:c,undirected:g=l}=s,h;"key"in s?(h=t?g?this.mergeUndirectedEdgeWithKey:this.mergeDirectedEdgeWithKey:g?this.addUndirectedEdgeWithKey:this.addDirectedEdgeWithKey,h.call(this,s.key,u,d,c)):(h=t?g?this.mergeUndirectedEdge:this.mergeDirectedEdge:g?this.addUndirectedEdge:this.addDirectedEdge,h.call(this,u,d,c))}}return this}nullCopy(n){let t=new e(Te({},this._options,n));return t.replaceAttributes(Te({},this.getAttributes())),t}emptyCopy(n){let t=this.nullCopy(n);return this._nodes.forEach((r,a)=>{let i=Te({},r.attributes);r=new t.NodeDataClass(a,i),t._nodes.set(a,r)}),t}copy(n){if(n=n||{},typeof n.type=="string"&&n.type!==this.type&&n.type!=="mixed")throw new Z(`Graph.copy: cannot create an incompatible copy from "${this.type}" type to "${n.type}" because this would mean losing information about the current graph.`);if(typeof n.multi=="boolean"&&n.multi!==this.multi&&n.multi!==!0)throw new Z("Graph.copy: cannot create an incompatible copy by downgrading a multi graph to a simple one because this would mean losing information about the current graph.");if(typeof n.allowSelfLoops=="boolean"&&n.allowSelfLoops!==this.allowSelfLoops&&n.allowSelfLoops!==!0)throw new Z("Graph.copy: cannot create an incompatible copy from a graph allowing self loops to one that does not because this would mean losing information about the current graph.");let t=this.emptyCopy(n),r=this._edges.values(),a,i;for(;a=r.next(),a.done!==!0;)i=a.value,$i(t,"copy",!1,i.undirected,i.key,i.source.key,i.target.key,Te({},i.attributes));return t}toJSON(){return this.export()}toString(){return"[object Graph]"}inspect(){let n={};this._nodes.forEach((i,o)=>{n[o]=i.attributes});let t={},r={};this._edges.forEach((i,o)=>{let s=i.undirected?"--":"->",l="",u=i.source.key,d=i.target.key,c;i.undirected&&u>d&&(c=u,u=d,d=c);let g=`(${u})${s}(${d})`;o.startsWith("geid_")?this.multi&&(typeof r[g]>"u"?r[g]=0:r[g]++,l+=`${r[g]}. `):l+=`[${o}]: `,l+=g,t[l]=i.attributes});let a={};for(let i in this)this.hasOwnProperty(i)&&!Ii.has(i)&&typeof this[i]!="function"&&typeof i!="symbol"&&(a[i]=this[i]);return a.attributes=this._attributes,a.nodes=n,a.edges=t,Be(a,"constructor",this.constructor),a}};typeof Symbol<"u"&&(de.prototype[Symbol.for("nodejs.util.inspect.custom")]=de.prototype.inspect);Cc.forEach(e=>{["add","merge","update"].forEach(n=>{let t=e.name(n),r=n==="add"?$i:Pc;e.generateKey?de.prototype[t]=function(a,i,o){return r(this,t,!0,(e.type||this.type)==="undirected",null,a,i,o,n==="update")}:de.prototype[t]=function(a,i,o,s){return r(this,t,!1,(e.type||this.type)==="undirected",a,i,o,s,n==="update")}})});Ou(de);Qu(de);pc(de);Ec(de);var lr=class extends de{constructor(n){let t=Te({type:"directed"},n);if("multi"in t&&t.multi!==!1)throw new V("DirectedGraph.from: inconsistent indication that the graph should be multi in given options!");if(t.type!=="directed")throw new V('DirectedGraph.from: inconsistent "'+t.type+'" type in given options!');super(t)}},ur=class extends de{constructor(n){let t=Te({type:"undirected"},n);if("multi"in t&&t.multi!==!1)throw new V("UndirectedGraph.from: inconsistent indication that the graph should be multi in given options!");if(t.type!=="undirected")throw new V('UndirectedGraph.from: inconsistent "'+t.type+'" type in given options!');super(t)}},cr=class extends de{constructor(n){let t=Te({multi:!0},n);if("multi"in t&&t.multi!==!0)throw new V("MultiGraph.from: inconsistent indication that the graph should be simple in given options!");super(t)}},dr=class extends de{constructor(n){let t=Te({type:"directed",multi:!0},n);if("multi"in t&&t.multi!==!0)throw new V("MultiDirectedGraph.from: inconsistent indication that the graph should be simple in given options!");if(t.type!=="directed")throw new V('MultiDirectedGraph.from: inconsistent "'+t.type+'" type in given options!');super(t)}},hr=class extends de{constructor(n){let t=Te({type:"undirected",multi:!0},n);if("multi"in t&&t.multi!==!0)throw new V("MultiUndirectedGraph.from: inconsistent indication that the graph should be simple in given options!");if(t.type!=="undirected")throw new V('MultiUndirectedGraph.from: inconsistent "'+t.type+'" type in given options!');super(t)}};function Kt(e){e.from=function(n,t){let r=Te({},n.options,t),a=new e(r);return a.import(n),a}}Kt(de);Kt(lr);Kt(ur);Kt(cr);Kt(dr);Kt(hr);de.Graph=de;de.DirectedGraph=lr;de.UndirectedGraph=ur;de.MultiGraph=cr;de.MultiDirectedGraph=dr;de.MultiUndirectedGraph=hr;de.InvalidArgumentsGraphError=V;de.NotFoundGraphError=H;de.UsageGraphError=Z;function vr(e,n){(n==null||n>e.length)&&(n=e.length);for(var t=0,r=Array(n);t<n;t++)r[t]=e[t];return r}function mr(e,n){if(e){if(typeof e=="string")return vr(e,n);var t={}.toString.call(e).slice(8,-1);return t==="Object"&&e.constructor&&(t=e.constructor.name),t==="Map"||t==="Set"?Array.from(e):t==="Arguments"||/^(?:Ui|I)nt(?:8|16|32)(?:Clamped)?Array$/.test(t)?vr(e,n):void 0}}function Ic(e){if(Array.isArray(e))return e}function Gc(e,n){var t=e==null?null:typeof Symbol<"u"&&e[Symbol.iterator]||e["@@iterator"];if(t!=null){var r,a,i,o,s=[],l=!0,u=!1;try{if(i=(t=t.call(e)).next,n===0){if(Object(t)!==t)return;l=!1}else for(;!(l=(r=i.call(t)).done)&&(s.push(r.value),s.length!==n);l=!0);}catch(d){u=!0,a=d}finally{try{if(!l&&t.return!=null&&(o=t.return(),Object(o)!==o))return}finally{if(u)throw a}}return s}}function zc(){throw new TypeError(`Invalid attempt to destructure non-iterable instance.
In order to be iterable, non-array objects must have a [Symbol.iterator]() method.`)}function re(e,n){return Ic(e)||Gc(e,n)||mr(e,n)||zc()}function U(e,n){var t=typeof Symbol<"u"&&e[Symbol.iterator]||e["@@iterator"];if(!t){if(Array.isArray(e)||(t=mr(e))||n&&e&&typeof e.length=="number"){t&&(e=t);var r=0,a=function(){};return{s:a,n:function(){return r>=e.length?{done:!0}:{done:!1,value:e[r++]}},e:function(l){throw l},f:a}}throw new TypeError(`Invalid attempt to iterate non-iterable instance.
In order to be iterable, non-array objects must have a [Symbol.iterator]() method.`)}var i,o=!0,s=!1;return{s:function(){t=t.call(e)},n:function(){var l=t.next();return o=l.done,l},e:function(l){s=!0,i=l},f:function(){try{o||t.return==null||t.return()}finally{if(s)throw i}}}}function Me(){return Float32Array.of(1,0,0,0,1,0,0,0,1)}function gr(e,n,t){return e[0]=n,e[4]=typeof t=="number"?t:n,e}function Vi(e,n){var t=Math.sin(n),r=Math.cos(n);return e[0]=r,e[1]=t,e[3]=-t,e[4]=r,e}function qi(e,n,t){return e[6]=n,e[7]=t,e}function ft(e,n){var t=e[0],r=e[1],a=e[2],i=e[3],o=e[4],s=e[5],l=e[6],u=e[7],d=e[8],c=n[0],g=n[1],h=n[2],f=n[3],y=n[4],v=n[5],p=n[6],m=n[7],b=n[8];return e[0]=c*t+g*i+h*l,e[1]=c*r+g*o+h*u,e[2]=c*a+g*s+h*d,e[3]=f*t+y*i+v*l,e[4]=f*r+y*o+v*u,e[5]=f*a+y*s+v*d,e[6]=p*t+m*i+b*l,e[7]=p*r+m*o+b*u,e[8]=p*a+m*s+b*d,e}function fa(e,n){var t=Math.cos(n),r=Math.sin(n);return{x:t*e.x+r*e.y,y:-r*e.x+t*e.y}}function Qt(e,n){var t=arguments.length>2&&arguments[2]!==void 0?arguments[2]:1,r=e[0],a=e[1],i=e[3],o=e[4],s=e[6],l=e[7],u=n.x,d=n.y;return{x:u*r+d*i+s*t,y:u*a+d*o+l*t}}function Bc(e,n){var t=e.height/e.width,r=n.height/n.width;return t<1&&r>1||t>1&&r<1?1:Math.min(Math.max(r,1/r),Math.max(1/t,t))}function gt(e,n,t,r,a){var i=e.angle,o=e.ratio,s=e.x,l=e.y,u=n.width,d=n.height,c=Me(),g=Math.min(u,d)-2*r,h=Bc(n,t);return a?(ft(c,qi(Me(),s,l)),ft(c,gr(Me(),o)),ft(c,Vi(Me(),i)),ft(c,gr(Me(),u/g/2/h,d/g/2/h))):(ft(c,gr(Me(),2*(g/u)*h,2*(g/d)*h)),ft(c,Vi(Me(),-i)),ft(c,gr(Me(),1/o)),ft(c,qi(Me(),-s,-l))),c}function Xi(e,n,t){var r=Qt(e,{x:Math.cos(n.angle),y:Math.sin(n.angle)},0),a=r.x,i=r.y;return 1/Math.sqrt(Math.pow(a,2)+Math.pow(i,2))/t.width}function ga(e,n,t,r,a){var i=Math.floor(n/a*r),o=Math.floor(e.drawingBufferHeight/a-t/a*r);return[i,o]}var Rn={black:"#000000",silver:"#C0C0C0",gray:"#808080",grey:"#808080",white:"#FFFFFF",maroon:"#800000",red:"#FF0000",purple:"#800080",fuchsia:"#FF00FF",green:"#008000",lime:"#00FF00",olive:"#808000",yellow:"#FFFF00",navy:"#000080",blue:"#0000FF",teal:"#008080",aqua:"#00FFFF",darkblue:"#00008B",mediumblue:"#0000CD",darkgreen:"#006400",darkcyan:"#008B8B",deepskyblue:"#00BFFF",darkturquoise:"#00CED1",mediumspringgreen:"#00FA9A",springgreen:"#00FF7F",cyan:"#00FFFF",midnightblue:"#191970",dodgerblue:"#1E90FF",lightseagreen:"#20B2AA",forestgreen:"#228B22",seagreen:"#2E8B57",darkslategray:"#2F4F4F",darkslategrey:"#2F4F4F",limegreen:"#32CD32",mediumseagreen:"#3CB371",turquoise:"#40E0D0",royalblue:"#4169E1",steelblue:"#4682B4",darkslateblue:"#483D8B",mediumturquoise:"#48D1CC",indigo:"#4B0082",darkolivegreen:"#556B2F",cadetblue:"#5F9EA0",cornflowerblue:"#6495ED",rebeccapurple:"#663399",mediumaquamarine:"#66CDAA",dimgray:"#696969",dimgrey:"#696969",slateblue:"#6A5ACD",olivedrab:"#6B8E23",slategray:"#708090",slategrey:"#708090",lightslategray:"#778899",lightslategrey:"#778899",mediumslateblue:"#7B68EE",lawngreen:"#7CFC00",chartreuse:"#7FFF00",aquamarine:"#7FFFD4",skyblue:"#87CEEB",lightskyblue:"#87CEFA",blueviolet:"#8A2BE2",darkred:"#8B0000",darkmagenta:"#8B008B",saddlebrown:"#8B4513",darkseagreen:"#8FBC8F",lightgreen:"#90EE90",mediumpurple:"#9370DB",darkviolet:"#9400D3",palegreen:"#98FB98",darkorchid:"#9932CC",yellowgreen:"#9ACD32",sienna:"#A0522D",brown:"#A52A2A",darkgray:"#A9A9A9",darkgrey:"#A9A9A9",lightblue:"#ADD8E6",greenyellow:"#ADFF2F",paleturquoise:"#AFEEEE",lightsteelblue:"#B0C4DE",powderblue:"#B0E0E6",firebrick:"#B22222",darkgoldenrod:"#B8860B",mediumorchid:"#BA55D3",rosybrown:"#BC8F8F",darkkhaki:"#BDB76B",mediumvioletred:"#C71585",indianred:"#CD5C5C",peru:"#CD853F",chocolate:"#D2691E",tan:"#D2B48C",lightgray:"#D3D3D3",lightgrey:"#D3D3D3",thistle:"#D8BFD8",orchid:"#DA70D6",goldenrod:"#DAA520",palevioletred:"#DB7093",crimson:"#DC143C",gainsboro:"#DCDCDC",plum:"#DDA0DD",burlywood:"#DEB887",lightcyan:"#E0FFFF",lavender:"#E6E6FA",darksalmon:"#E9967A",violet:"#EE82EE",palegoldenrod:"#EEE8AA",lightcoral:"#F08080",khaki:"#F0E68C",aliceblue:"#F0F8FF",honeydew:"#F0FFF0",azure:"#F0FFFF",sandybrown:"#F4A460",wheat:"#F5DEB3",beige:"#F5F5DC",whitesmoke:"#F5F5F5",mintcream:"#F5FFFA",ghostwhite:"#F8F8FF",salmon:"#FA8072",antiquewhite:"#FAEBD7",linen:"#FAF0E6",lightgoldenrodyellow:"#FAFAD2",oldlace:"#FDF5E6",magenta:"#FF00FF",deeppink:"#FF1493",orangered:"#FF4500",tomato:"#FF6347",hotpink:"#FF69B4",coral:"#FF7F50",darkorange:"#FF8C00",lightsalmon:"#FFA07A",orange:"#FFA500",lightpink:"#FFB6C1",pink:"#FFC0CB",gold:"#FFD700",peachpuff:"#FFDAB9",navajowhite:"#FFDEAD",moccasin:"#FFE4B5",bisque:"#FFE4C4",mistyrose:"#FFE4E1",blanchedalmond:"#FFEBCD",papayawhip:"#FFEFD5",lavenderblush:"#FFF0F5",seashell:"#FFF5EE",cornsilk:"#FFF8DC",lemonchiffon:"#FFFACD",floralwhite:"#FFFAF0",snow:"#FFFAFA",lightyellow:"#FFFFE0",ivory:"#FFFFF0"},Yi=new Int8Array(4),pr=new Int32Array(Yi.buffer,0,1),Ji=new Float32Array(Yi.buffer,0,1),Nc=/^\s*rgba?\s*\(/,Mc=/^\s*rgba?\s*\(\s*([0-9]*)\s*,\s*([0-9]*)\s*,\s*([0-9]*)(?:\s*,\s*(.*)?)?\)\s*$/;function br(e){var n=0,t=0,r=0,a=1,i=e.toLowerCase();if(i==="transparent")return{r:0,g:0,b:0,a:0};if(i in Rn)return br(Rn[i]);if(e[0]==="#")e.length===4?(n=parseInt(e.charAt(1)+e.charAt(1),16),t=parseInt(e.charAt(2)+e.charAt(2),16),r=parseInt(e.charAt(3)+e.charAt(3),16)):(n=parseInt(e.charAt(1)+e.charAt(2),16),t=parseInt(e.charAt(3)+e.charAt(4),16),r=parseInt(e.charAt(5)+e.charAt(6),16)),e.length===9&&(a=parseInt(e.charAt(7)+e.charAt(8),16)/255);else if(Nc.test(e)){var o=e.match(Mc);o&&(n=+o[1],t=+o[2],r=+o[3],o[4]&&(a=+o[4]))}return{r:n,g:t,b:r,a}}function Ki(e){var n=arguments.length>1&&arguments[1]!==void 0?arguments[1]:!1,t=br(e),r=t.r,a=t.g,i=t.b,o=t.a;return n?[r/255*o,a/255*o,i/255*o,o]:[r/255,a/255,i/255,o]}var Zt={};for(Sn in Rn)Zt[Sn]=We(Rn[Sn]),Zt[Rn[Sn]]=Zt[Sn];var Sn;function Wc(e,n,t,r,a){return pr[0]=r<<24|t<<16|n<<8|e,a&&(pr[0]=pr[0]&4278190079),Ji[0]}function We(e){if(e=e.toLowerCase(),typeof Zt[e]<"u")return Zt[e];var n=br(e),t=n.r,r=n.g,a=n.b,i=n.a;i=i*255|0;var o=Wc(t,r,a,i,!0);return Zt[e]=o,o}function pt(e,n){Ji[0]=We(e);var t=pr[0];n&&(t=t|16777216);var r=t&255,a=t>>8&255,i=t>>16&255,o=t>>24&255;return[r,a,i,o]}function en(e){var n=e>>>8&255,t=e>>>16&255,r=e>>>24&255,a=e&255;return(a<<24|r<<16|t<<8|n)>>>0}function pa(e,n,t,r){return(t<<24|n<<16|e<<8|r)>>>0}function Zi(e,n,t,r,a,i){var o=ga(e,t,r,a,i),s=re(o,2),l=s[0],u=s[1],d=new Uint8Array(4);e.bindFramebuffer(e.FRAMEBUFFER,n),e.readPixels(l,u,1,1,e.RGBA,e.UNSIGNED_BYTE,d);var c=re(d,4),g=c[0],h=c[1],f=c[2],y=c[3];return[g,h,f,y]}function Qi(e){var n=br(e),t=n.r,r=n.g,a=n.b,i=n.a,o=(t/255).toFixed(6),s=(r/255).toFixed(6),l=(a/255).toFixed(6),u=i.toFixed(6);return"vec4(".concat(o,", ").concat(s,", ").concat(l,", ").concat(u,")")}function Uc(e,n){if(typeof e!="object"||!e)return e;var t=e[Symbol.toPrimitive];if(t!==void 0){var r=t.call(e,n||"default");if(typeof r!="object")return r;throw new TypeError("@@toPrimitive must return a primitive value.")}return(n==="string"?String:Number)(e)}function no(e){var n=Uc(e,"string");return typeof n=="symbol"?n:n+""}function D(e,n,t){return(n=no(n))in e?Object.defineProperty(e,n,{value:t,enumerable:!0,configurable:!0,writable:!0}):e[n]=t,e}function eo(e,n){var t=Object.keys(e);if(Object.getOwnPropertySymbols){var r=Object.getOwnPropertySymbols(e);n&&(r=r.filter(function(a){return Object.getOwnPropertyDescriptor(e,a).enumerable})),t.push.apply(t,r)}return t}function O(e){for(var n=1;n<arguments.length;n++){var t=arguments[n]!=null?arguments[n]:{};n%2?eo(Object(t),!0).forEach(function(r){D(e,r,t[r])}):Object.getOwnPropertyDescriptors?Object.defineProperties(e,Object.getOwnPropertyDescriptors(t)):eo(Object(t)).forEach(function(r){Object.defineProperty(e,r,Object.getOwnPropertyDescriptor(t,r))})}return e}function ee(e,n){if(!(e instanceof n))throw new TypeError("Cannot call a class as a function")}function to(e,n){for(var t=0;t<n.length;t++){var r=n[t];r.enumerable=r.enumerable||!1,r.configurable=!0,"value"in r&&(r.writable=!0),Object.defineProperty(e,no(r.key),r)}}function te(e,n,t){return n&&to(e.prototype,n),t&&to(e,t),Object.defineProperty(e,"prototype",{writable:!1}),e}function Ct(e){return Ct=Object.setPrototypeOf?Object.getPrototypeOf.bind():function(n){return n.__proto__||Object.getPrototypeOf(n)},Ct(e)}function ro(){try{var e=!Boolean.prototype.valueOf.call(Reflect.construct(Boolean,[],function(){}))}catch{}return(ro=function(){return!!e})()}function Oc(e){if(e===void 0)throw new ReferenceError("this hasn't been initialised - super() hasn't been called");return e}function Hc(e,n){if(n&&(typeof n=="object"||typeof n=="function"))return n;if(n!==void 0)throw new TypeError("Derived constructors may only return object or undefined");return Oc(e)}function ge(e,n,t){return n=Ct(n),Hc(e,ro()?Reflect.construct(n,t||[],Ct(e).constructor):n.apply(e,t))}function va(e,n){return va=Object.setPrototypeOf?Object.setPrototypeOf.bind():function(t,r){return t.__proto__=r,t},va(e,n)}function pe(e,n){if(typeof n!="function"&&n!==null)throw new TypeError("Super expression must either be null or a function");e.prototype=Object.create(n&&n.prototype,{constructor:{value:e,writable:!0,configurable:!0}}),Object.defineProperty(e,"prototype",{writable:!1}),n&&va(e,n)}function jc(e){if(Array.isArray(e))return vr(e)}function $c(e){if(typeof Symbol<"u"&&e[Symbol.iterator]!=null||e["@@iterator"]!=null)return Array.from(e)}function Vc(){throw new TypeError(`Invalid attempt to spread non-iterable instance.
In order to be iterable, non-array objects must have a [Symbol.iterator]() method.`)}function K(e){return jc(e)||$c(e)||mr(e)||Vc()}function ne(e){"@babel/helpers - typeof";return ne=typeof Symbol=="function"&&typeof Symbol.iterator=="symbol"?function(n){return typeof n}:function(n){return n&&typeof Symbol=="function"&&n.constructor===Symbol&&n!==Symbol.prototype?"symbol":typeof n},ne(e)}function lt(e){return ne(e)==="object"&&e!==null&&"attribute"in e}function ao(){var e=`
float sdf_circle(vec2 uv, float size) {
  return length(uv) - size;
}
`;return{name:"circle",glsl:e,uniforms:[]}}function io(e){var n,t=WebGL2RenderingContext,r=t.UNSIGNED_BYTE,a=(n=e?.color)!==null&&n!==void 0?n:{attribute:"color"};if(!lt(a)){var i=a,o=`
vec4 layer_fill() {
  return `.concat(Qi(i),`;
}
`);return{name:"fill",uniforms:[],attributes:[],glsl:o}}var s=a.attribute,l=`
vec4 layer_fill(vec4 v_fillColor) {
  return v_fillColor;
}
`;return{name:"fill",uniforms:[],attributes:[{name:"fillColor",size:4,type:r,normalized:!0,source:s}],glsl:l}}function qc(e,n){if(typeof e=="string"){var t=Ki(e).map(function(u){return u.toFixed(6)}),r=re(t,4),a=r[0],i=r[1],o=r[2],s=r[3];return{glsl:"vec4(".concat(a,", ").concat(i,", ").concat(o,", ").concat(s,")"),attributes:[],needsNodeColors:!1}}if("node"in e&&e.node)return{glsl:e.node==="source"?"v_sourceColor":"v_targetColor",attributes:[],needsNodeColors:!0};var l=e;return{glsl:"v_".concat(n),attributes:[{name:"a_".concat(n),size:4,type:WebGL2RenderingContext.UNSIGNED_BYTE,normalized:!0,source:l.attribute,defaultValue:l.default}],needsNodeColors:!1}}function Ft(e){var n=e?.color;if(n===void 0){var t=`
// Plain solid color layer
vec4 layer_plain(EdgeContext ctx) {
  return v_color;
}
`;return{name:"plain",glsl:t,uniforms:[],attributes:[]}}var r=qc(n,"plainColor"),a=`
// Plain solid color layer
vec4 layer_plain(EdgeContext ctx) {
  return `.concat(r.glsl,`;
}
`);return{name:"plain",glsl:a,uniforms:[],attributes:r.attributes,needsNodeColors:r.needsNodeColors}}function oo(){var e=`
// Position at parameter t \u2208 [0, 1]
vec2 path_straight_position(float t, vec2 source, vec2 target) {
  return mix(source, target, t);
}

// Total length of the path (analytical - more efficient than sampling)
float path_straight_length(vec2 source, vec2 target) {
  return length(target - source);
}
`;return{name:"straight",segments:1,minBodyLengthRatio:0,linearParameterization:!0,glsl:e,uniforms:[],attributes:[]}}function so(e){var n=e??{},t=n.segments,r=t===void 0?32:t,a=`
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
`;return{name:"loop",segments:r,glsl:a,needsNodeSize:!0,uniforms:[],attributes:[{name:"loopRadius",size:1,type:WebGL2RenderingContext.FLOAT},{name:"loopAngle",size:1,type:WebGL2RenderingContext.FLOAT},{name:"loopSpread",size:1,type:WebGL2RenderingContext.FLOAT},{name:"loopFixedOrientation",size:1,type:WebGL2RenderingContext.FLOAT}],variables:{loopRadius:{type:"number",default:4},loopAngle:{type:"number",default:Math.PI/4},loopSpread:{type:"number",default:80*Math.PI/180},loopFixedOrientation:{type:"number",default:0}},spread:{variable:"loopRadius",compute:function(o){return o+4}}}}var go=it(_n());function lo(e){return ne(e)==="object"&&"glsl"in e&&!("attributes"in e)}function uo(e){return ne(e)==="object"&&"glsl"in e&&!("attributes"in e)}var An={shapes:[ao()],variables:{},layers:[io()],label:{},backdrop:{},labelAttachments:{}},tn={paths:[oo(),so()],extremities:[],variables:{},layers:[Ft()],defaultHead:"none",defaultTail:"none",label:{}},yr=["nodes","topNodes"],_r=["edges","topEdges"],xr=[].concat(_r,yr),co={nodes:An,edges:tn,depthLayers:K(xr)};var Xc=function(n){return n},Yc=function(n){return n*n},Jc=function(n){return n*(2-n)},Kc=function(n){return(n*=2)<1?.5*n*n:-.5*(--n*(n-2)-1)},Zc=function(n){return n*n*n},Qc=function(n){return--n*n*n+1},ed=function(n){return(n*=2)<1?.5*n*n*n:.5*((n-=2)*n*n+2)},td=function(n){return n===0?0:Math.pow(2,10*(n-1))},nd=function(n){return n===1?1:1-Math.pow(2,-10*n)},rd=function(n){return n===0?0:n===1?1:n<.5?Math.pow(2,10*(2*n-1))/2:(2-Math.pow(2,-10*(2*n-1)))/2},ma={linear:Xc,quadraticIn:Yc,quadraticOut:Jc,quadraticInOut:Kc,cubicIn:Zc,cubicOut:Qc,cubicInOut:ed,exponentialIn:td,exponentialOut:nd,exponentialInOut:rd};function Tr(e){return e?typeof e=="function"?e:ma[e]:ma.linear}function ya(e){return ne(e)==="object"&&e!==null&&"attribute"in e}function po(e){return typeof e=="function"}function vo(e){return ne(e)==="object"&&e!==null&&"when"in e&&typeof e.when=="function"&&"then"in e}function mo(e){return ne(e)==="object"&&e!==null&&"whenState"in e&&"then"in e}function bo(e){return ne(e)==="object"&&e!==null&&"whenData"in e&&"then"in e}var ad={isHovered:!1,isLabelHovered:!1,isHidden:!1,isHighlighted:!1,isDragged:!1},id={isHovered:!1,isLabelHovered:!1,isHidden:!1,isHighlighted:!1,parallelIndex:0,parallelCount:1},od={isIdle:!0,isPanning:!1,isZooming:!1,isDragging:!1,hasHovered:!1,hasHighlighted:!1};function yo(e){return O(O({},ad),e)}function _o(e){return O(O({},id),e)}function _a(e){return O(O({},od),e)}var sd={x:{attribute:"x"},y:{attribute:"y"},size:{whenState:"isHovered",then:{attribute:"size",defaultValue:12},else:{attribute:"size",defaultValue:10}},color:{attribute:"color",defaultValue:"#666"},label:{attribute:"label"},visibility:{whenState:"isHidden",then:"hidden",else:"visible"},labelVisibility:{whenState:"isHovered",then:"visible",else:"auto"},backdropVisibility:{whenState:"isHovered",then:"visible",else:"hidden"},backdropColor:"#ffffff",backdropShadowColor:"rgba(0, 0, 0, 0.5)",backdropShadowBlur:12,backdropPadding:6},ld={size:{attribute:"size",defaultValue:1},color:{attribute:"color",defaultValue:"#ccc"},label:{attribute:"label"},visibility:{whenState:"isHidden",then:"hidden",else:"visible"}};var Sr={nodes:O(O({},sd),{},{depth:{whenState:"isHovered",then:"topNodes",else:"nodes"}}),edges:O(O({},ld),{},{depth:{whenState:["isHighlighted","isHovered"],then:"topEdges",else:"edges"}})};function ud(e){return"min"in e||"max"in e||"minValue"in e||"maxValue"in e||"easing"in e}function cd(e){return"dict"in e}function Rr(e,n){return typeof e=="string"?n[e]===!0:Array.isArray(e)?e.every(function(t){return n[t]===!0}):ne(e)==="object"&&e!==null?Object.entries(e).every(function(t){var r=re(t,2),a=r[0],i=r[1];return n[a]===i}):!1}function dd(e,n){var t=n[e.attribute];return t===void 0?e.defaultValue:t}function hd(e,n,t){var r,a,i,o,s=n[e.attribute];if(s===void 0)return e.defaultValue;var l=Number(s);if(isNaN(l))return e.defaultValue;if(e.min===void 0&&e.max===void 0)return l;var u=(r=e.minValue)!==null&&r!==void 0?r:l,d=(a=e.maxValue)!==null&&a!==void 0?a:l;if(d===u){var c;return(c=e.min)!==null&&c!==void 0?c:l}var g=(l-u)/(d-u);g=Math.max(0,Math.min(1,g));var h=Tr(e.easing);g=h(g);var f=(i=e.min)!==null&&i!==void 0?i:0,y=(o=e.max)!==null&&o!==void 0?o:1;return f+g*(y-f)}function fd(e,n){var t=n[e.attribute];if(t===void 0)return e.defaultValue;var r=String(t);return r in e.dict?e.dict[r]:e.defaultValue}function gd(e,n,t){return cd(e)?fd(e,n):ud(e)?hd(e,n):dd(e,n)}function xo(e,n){return typeof e=="string"?!!n[e]:Array.isArray(e)?e.every(function(t){return!!n[t]}):ne(e)==="object"&&e!==null?Object.entries(e).every(function(t){var r=re(t,2),a=r[0],i=r[1];return n[a]===i}):!1}function Er(e,n,t,r,a,i){if(e==null)return i;if(ne(e)!=="object"&&typeof e!="function")return e;if(vo(e)){var o=e.when(n,t,r,a)?e.then:e.else;return o===void 0?i:Er(o,n,t,r,a,i)}if(mo(e)){var s=Rr(e.whenState,t)?e.then:e.else;return s===void 0?i:Er(s,n,t,r,a,i)}if(bo(e)){var l=xo(e.whenData,n)?e.then:e.else;return l===void 0?i:Er(l,n,t,r,a,i)}if(po(e)){var u=e(n,t,r,a);return u??i}if(ya(e)){var d=gd(e,n);return d??i}return e}function Xe(e){if(e==null)return"static";if(vo(e))return"graph-state";if(mo(e)){var n=Xe(e.then),t=e.else!==void 0?Xe(e.else):"static";return Ue("item-state",Ue(n,t))}if(bo(e)){var r=Xe(e.then),a=e.else!==void 0?Xe(e.else):"static";return Ue(r,a)}return po(e)?"graph-state":(ya(e),"static")}function Ue(e,n){return e==="graph-state"||n==="graph-state"?"graph-state":e==="item-state"||n==="item-state"?"item-state":"static"}function xa(e){if(!e)return{dependency:"static",xAttribute:null,yAttribute:null};var n=Array.isArray(e)?e:[e],t="static",r=null,a=null,i=U(n),o;try{for(i.s();!(o=i.n()).done;){var s=o.value;if("matchData"in s&&"cases"in s)for(var l=s.cases,u=0,d=Object.values(l);u<d.length;u++)for(var c=d[u],g=0,h=Object.values(c);g<h.length;g++){var f=h[g];t=Ue(t,Xe(f))}else if("matchState"in s&&"cases"in s){t=Ue(t,"item-state");for(var y=s.cases,v=0,p=Object.values(y);v<p.length;v++)for(var m=p[v],b=0,x=Object.values(m);b<x.length;b++){var _=x[b];t=Ue(t,Xe(_))}}else if("when"in s&&"then"in s)t="graph-state";else if("whenState"in s){t=Ue(t,"item-state");var T=s.then;if(T&&ne(T)==="object")for(var R=0,A=Object.values(T);R<A.length;R++){var S=A[R];t=Ue(t,Xe(S))}var k=s.else;if(k&&ne(k)==="object")for(var L=0,C=Object.values(k);L<C.length;L++){var z=C[L];t=Ue(t,Xe(z))}}else if("whenData"in s){var F=s.then;if(F&&ne(F)==="object")for(var P=0,N=Object.values(F);P<N.length;P++){var G=N[P];t=Ue(t,Xe(G))}var E=s.else;if(E&&ne(E)==="object")for(var I=0,W=Object.values(E);I<W.length;I++){var B=W[I];t=Ue(t,Xe(B))}}else for(var w=0,$=Object.entries(s);w<$.length;w++){var j=re($[w],2),J=j[0],Se=j[1];if(!Ea.has(J)&&(t=Ue(t,Xe(Se)),(J==="x"||J==="y")&&ya(Se))){var le=Se.attribute;J==="x"&&!r&&(r=le),J==="y"&&!a&&(a=le)}}}}catch(Y){i.e(Y)}finally{i.f()}return{dependency:t,xAttribute:r,yAttribute:a}}var ut={size:10,color:"#666",opacity:1,shape:"circle",rotationAlignment:"viewport",labelRotationAlignment:"viewport",visibility:"visible",depth:"nodes",zIndex:0,label:"",labelColor:"#000",labelSize:12,labelFont:"sans-serif",labelVisibility:"auto",labelPosition:"right",labelAngle:0,labelDepth:"nodes",backdropVisibility:"hidden",backdropColor:"transparent",backdropShadowColor:"transparent",backdropShadowBlur:0,backdropPadding:0,backdropBorderColor:"transparent",backdropBorderWidth:0,backdropCornerRadius:0,backdropLabelPadding:-1,backdropArea:"both",labelAttachment:null,labelAttachmentPlacement:"below"},Ta={size:1,color:"#ccc",opacity:1,path:"straight",selfLoopPath:"loop",parallelSpread:.25,tail:"none",head:"none",visibility:"visible",depth:"edges",zIndex:0,label:"",labelColor:"#666",labelVisibility:"auto",labelDepth:"edges"},Ea=new Set(["when","whenState","whenData","then","else"]),ho=Object.keys(ut),pd=Object.values(ut),fo=Object.keys(Ta),vd=Object.values(Ta);function nn(e,n,t,r,a,i,o){for(var s in n){var l;if(!Ea.has(s)){var u=(l=e[s])!==null&&l!==void 0?l:o[s];e[s]=Er(n[s],t,r,a,i,u)}}"depth"in n&&!("labelDepth"in n)&&(e.labelDepth=e.depth)}function To(e,n,t,r,a,i,o){var s=U(n),l;try{for(s.s();!(l=s.n()).done;){var u=l.value;if("matchData"in u&&"cases"in u){var d,c=String((d=t[u.matchData])!==null&&d!==void 0?d:""),g=u.cases;c in g&&nn(e,g[c],t,r,a,i,o)}else if("matchState"in u&&"cases"in u){var h,f=String((h=r[u.matchState])!==null&&h!==void 0?h:""),y=u.cases;f in y&&nn(e,y[f],t,r,a,i,o)}else if("when"in u&&"then"in u){var v=u.when;if(!v(t,r,a,i))continue;nn(e,u.then,t,r,a,i,o)}else if("whenState"in u&&"then"in u){if(!Rr(u.whenState,r))continue;nn(e,u.then,t,r,a,i,o)}else if("whenData"in u&&"then"in u){if(!xo(u.whenData,t))continue;nn(e,u.then,t,r,a,i,o)}else nn(e,u,t,r,a,i,o)}}catch(p){s.e(p)}finally{s.f()}}function Sa(e,n,t,r,a,i){for(var o=i||{},s=o,l=0,u=ho.length;l<u;l++)s[ho[l]]=pd[l];if(s.x=void 0,s.y=void 0,s.labelBackgroundColor=void 0,s.labelBackgroundPadding=void 0,s.labelCursor=void 0,!e){var d,c,g,h,f;return s.x=(d=n.x)!==null&&d!==void 0?d:0,s.y=(c=n.y)!==null&&c!==void 0?c:0,o.size=(g=n.size)!==null&&g!==void 0?g:10,o.color=(h=n.color)!==null&&h!==void 0?h:"#666",o.label=(f=n.label)!==null&&f!==void 0?f:"",o}var y=Array.isArray(e)?e:[e];return To(s,y,n,t,r,a,ut),o}function Ra(e,n,t,r,a,i){for(var o=i||{},s=o,l=0,u=fo.length;l<u;l++)s[fo[l]]=vd[l];if(!e){var d,c,g;return o.size=(d=n.size)!==null&&d!==void 0?d:1,o.color=(c=n.color)!==null&&c!==void 0?c:"#ccc",o.label=(g=n.label)!==null&&g!==void 0?g:"",o}var h=Array.isArray(e)?e:[e];return To(s,h,n,t,r,a,Ta),typeof s.labelPosition=="number"&&(s.labelPosition=void 0),o}function md(e,n,t){var r,a;if(e==null)return t;if(typeof e=="function")return(r=e(n))!==null&&r!==void 0?r:t;if(ne(e)==="object"&&"when"in e){var i,o=e,s=o.when(n),l=s?o.then:o.else;return l===void 0?t:typeof l=="function"?(i=l(n))!==null&&i!==void 0?i:t:l??t}if(ne(e)==="object"&&"whenState"in e){var u,d=e,c=Rr(d.whenState,n),g=c?d.then:d.else;return g===void 0?t:typeof g=="function"?(u=g(n))!==null&&u!==void 0?u:t:g??t}return(a=e)!==null&&a!==void 0?a:t}function Aa(e,n){var t={};if(!e)return t;var r=Array.isArray(e)?e:[e],a=U(r),i;try{for(a.s();!(i=a.n()).done;){var o=i.value;if("when"in o&&"then"in o){var s=o.when(n),l=s?o.then:o.else;l&&ne(l)==="object"&&ba(t,l,n)}else if("whenState"in o&&"then"in o){var u=Rr(o.whenState,n),d=u?o.then:o.else;d&&ne(d)==="object"&&ba(t,d,n)}else ba(t,o,n)}}catch(c){a.e(c)}finally{a.f()}return t}function ba(e,n,t){for(var r=0,a=Object.entries(n);r<a.length;r++){var i=re(a[r],2),o=i[0],s=i[1];Ea.has(o)||(e[o]=md(s,t))}}var Ar=(function(e){function n(){var t;return ee(this,n),t=ge(this,n),t.rawEmitter=t,t}return pe(n,e),te(n)})(go.EventEmitter);var Eo=co;function Da(e,n){var t=n.size;if(t!==0){var r=e.length;e.length+=t;var a=0;n.forEach(function(i){e[r+a]=i,a++})}}function So(e){e=e||{};for(var n=0,t=arguments.length<=1?0:arguments.length-1;n<t;n++){var r=n+1<1||arguments.length<=n+1?void 0:arguments[n+1];r&&Object.assign(e,r)}return e}function rn(e,n){for(var t in n)if(Object.prototype.hasOwnProperty.call(n,t)&&n[t]!==e[t])return!0;return!1}function Ro(e,n){var t=e,r=n;for(var a in t)if(Object.prototype.hasOwnProperty.call(t,a)&&t[a]!==r[a])return!1;for(var i in r)if(Object.prototype.hasOwnProperty.call(r,i)&&r[i]!==t[i])return!1;return!0}function Lt(e,n,t){t?e.add(n):e.delete(n)}var Do=it(an());var wo={easing:"quadraticInOut",duration:150};function vt(e){return e.labelVisibility==="visible"&&e.visibility!=="hidden"}function wa(e){return e.backdropVisibility==="visible"&&e.visibility!=="hidden"}function Dr(e){return[e.rotationAlignment==="graph"?1:0,e.labelRotationAlignment==="graph"?1:0]}function ka(e,n,t){var r=e[n];if(r)for(var a=0;a<r.length;a++){var i=r[a];if(!(t<i.offset||t>=i.offset+i.count)){if(i.count===1)r.splice(a,1);else if(t===i.offset)i.offset++,i.count--;else if(t===i.offset+i.count-1)i.count--;else{var o=t+1,s=i.offset+i.count-o;i.count=t-i.offset,r.splice(a+1,0,{offset:o,count:s})}return}}}function Ca(e,n,t){if(!e[n]){e[n]=[{offset:t,count:1}];return}for(var r=e[n],a=r.length,i=0;i<r.length;i++)if(t<r[i].offset){a=i;break}var o=a>0?r[a-1]:null,s=a<r.length?r[a]:null,l=o&&o.offset+o.count===t,u=s&&t+1===s.offset;l&&u?(o.count+=1+s.count,r.splice(a,1)):l?o.count++:u?(s.offset--,s.count++):r.splice(a,0,{offset:t,count:1})}function ko(e){if(!(0,Do.default)(e))throw new Error("Sigma: invalid graph instance.")}function wr(e){var n=re(e.x,2),t=n[0],r=n[1],a=re(e.y,2),i=a[0],o=a[1],s=Math.max(r-t,o-i),l=(r+t)/2,u=(o+i)/2;(s===0||Math.abs(s)===1/0||isNaN(s))&&(s=1),isNaN(l)&&(l=0),isNaN(u)&&(u=0);var d=function(g){return{x:.5+(g.x-l)/s,y:.5+(g.y-u)/s}};return d.applyTo=function(c){c.x=.5+(c.x-l)/s,c.y=.5+(c.y-u)/s},d.inverse=function(c){return{x:l+s*(c.x-.5),y:u+s*(c.y-.5)}},d.ratio=s,d}var kr={hideEdgesOnMove:!1,hideLabelsOnMove:!1,renderLabels:!0,renderEdgeLabels:!1,edgeLabelAnchors:"nodeLabels",enableEdgeEvents:!1,nodeLabelEvents:!1,edgeLabelEvents:!1,pickingDownSizingRatio:2,nodePickingPadding:0,edgePickingPadding:4,labelPickingPadding:10,stagePadding:30,minEdgeThickness:1.7,antiAliasingFeather:1,antialiasEdges:!0,antialiasNodes:!0,dragTimeout:100,draggedEventsTolerance:3,inertiaDuration:200,inertiaRatio:3,zoomDuration:250,zoomingRatio:1.7,doubleClickTimeout:300,doubleClickZoomingRatio:2.2,doubleClickZoomingDuration:200,tapMoveTolerance:10,zoomToSizeRatioFunction:Math.sqrt,itemSizesReference:"positions",autoRescale:!0,autoRescaleContent:"positions",enableNodeDrag:!1,getDraggedNodes:function(n){return[n]},dragPositionToAttributes:null,labelDensity:1,labelGridCellSize:100,labelRenderedSizeThreshold:6,labelPixelSnapping:!0,minCameraRatio:null,maxCameraRatio:null,enableCameraZooming:!0,enableCameraPanning:!0,enableCameraRotation:!0,enableCameraMouseRotation:!0,cameraPanBoundaries:null,gestureTarget:"graph",sharedGestureWheelMessage:"Use Ctrl + scroll to zoom the graph",sharedGestureAppleWheelMessage:"Use \u2318 + scroll to zoom the graph",sharedGestureTouchMessage:"Use two fingers to move the graph",allowInvalidContainer:!1,DEBUG_displayPickingLayer:!1,DEBUG_logShaders:!1,DEBUG_logRenderStats:!1,DEBUG_gpuTimerQueries:!1};function Cr(e){if(typeof e.labelDensity!="number"||e.labelDensity<0)throw new Error("Settings: invalid `labelDensity`. Expecting a positive number.");var n=e.minCameraRatio,t=e.maxCameraRatio;if(typeof n=="number"&&typeof t=="number"&&t<n)throw new Error("Settings: invalid camera ratio boundaries. Expecting `maxCameraRatio` to be greater than `minCameraRatio`.")}function Co(e){return So({},kr,e)}var bd=new Set(["italic","oblique"]),yd=new Set(["bold","bolder","lighter"]);function Fr(e){for(var n="normal",t="normal",r=e.trim(),a=r.split(/\s+/),i=0,o=0;o<a.length;o++){var s=a[o].toLowerCase();if(bd.has(s))t=s,i=o+1;else if(yd.has(s)||/^\d{3}$/.test(s))n=s,i=o+1;else if(s==="normal")i=o+1;else break}var l=a.slice(i).join(" ");return{family:l||r,weight:n,style:t}}function on(e,n,t){var r=document.createElement(e);if(n)for(var a in n)r.style[a]=n[a];if(t)for(var i in t)r.setAttribute(i,t[i]);return r}function Dn(){return typeof window.devicePixelRatio<"u"?window.devicePixelRatio:1}var Wo=it(_n());function _d(e){var n=e.type===WebGL2RenderingContext.FLOAT&&!e.normalized,t=e.type===WebGL2RenderingContext.UNSIGNED_BYTE&&e.size===4&&e.normalized;if(!n&&!t)throw new Error('Attribute "'.concat(e.name,'" is invalid: only non-normalized FLOAT and normalized UNSIGNED_BYTE with size 4 are supported.'));return e.normalized?1:e.size}function Fa(e){var n=0;return e.forEach(function(t){return n+=_d(t)}),n}function Uo(e,n,t){var r=e==="VERTEX"?n.VERTEX_SHADER:n.FRAGMENT_SHADER,a=n.createShader(r);if(a===null)throw new Error("loadShader: error while creating the shader");n.shaderSource(a,t),n.compileShader(a);var i=n.getShaderParameter(a,n.COMPILE_STATUS);if(!i){var o=n.getShaderInfoLog(a);throw n.deleteShader(a),new Error(`loadShader: error while compiling the shader:
`.concat(o,`
`).concat(t))}return a}function Pa(e,n){return Uo("VERTEX",e,n)}function Ia(e,n){return Uo("FRAGMENT",e,n)}function Ga(e,n){var t=e.createProgram();if(t===null)throw new Error("loadProgram: error while creating the program.");var r,a;for(r=0,a=n.length;r<a;r++)e.attachShader(t,n[r]);e.linkProgram(t);var i=e.getProgramParameter(t,e.LINK_STATUS);if(!i){var o=e.getProgramInfoLog(t);throw e.deleteProgram(t),new Error("loadProgram: error while linking the program: ".concat(o))}return t}function Fo(e){var n=e.gl,t=e.buffer,r=e.program,a=e.vertexShader,i=e.fragmentShader;n.deleteShader(a),n.deleteShader(i),n.deleteProgram(r),n.deleteBuffer(t)}function ye(e){return e%1===0?e.toFixed(1):e.toString()}var Br=new Map,Oo=new Map,xd=0;function Ho(e,n){var t,r=(t=e.variables)===null||t===void 0||(t=t[n.source||n.name.replace(/^a_/,"")])===null||t===void 0?void 0:t.default,a=typeof r=="number"?r:typeof n.defaultValue=="number"?n.defaultValue:0;return ye(a)}function Nr(e,n,t){var r,a=arguments.length>3&&arguments[3]!==void 0?arguments[3]:function(o){return"v_".concat(o.name.replace(/^a_/,""))},i=[n,t].concat(K(e.uniforms.filter(function(o){return o.type==="float"}).map(function(o){var s;return ye((s=o.value)!==null&&s!==void 0?s:0)})),K(((r=e.attributes)!==null&&r!==void 0?r:[]).map(a)));return"sdf_".concat(e.name,"(").concat(i.join(", "),")")}function Td(e){var n,t=e.name,r=e.uniforms.filter(function(o){return o.type==="float"&&o.value!==void 0&&o.value!==0}).map(function(o){return"".concat(o.name.replace("u_",""),"=").concat(o.value)}).sort(),a=((n=e.attributes)!==null&&n!==void 0?n:[]).map(function(o){var s;return"".concat(o.name,"@").concat((s=o.source)!==null&&s!==void 0?s:"","=").concat(Ho(e,o))}),i=[].concat(K(r),K(a));return i.length>0&&(t+="#"+i.join("#")),t}function Ed(e){var n=Td(e);if(!Br.has(n)){var t={},r=U(e.uniforms),a;try{for(r.s();!(a=r.n()).done;){var i=a.value;i.type==="float"&&i.value!==void 0&&(t[i.name]=i.value)}}catch(o){r.e(o)}finally{r.f()}Br.set(n,{shape:e,uniformValues:t,slug:n}),Oo.set(n,xd++)}return n}function Ln(e){var n;return(n=Oo.get(e))!==null&&n!==void 0?n:-1}function jo(e){var n=[],t=new Set,r=new Set,a=/mat2 rotate2D\(float angle\)\s*\{[^}]+\}/,i=U(e),o;try{for(i.s();!(o=i.n()).done;){var s=o.value;if(!r.has(s.name)){r.add(s.name);var l=s.glsl;a.test(l)&&(t.has("rotate2D")?l=l.replace(a,""):t.add("rotate2D")),n.push(l)}}}catch(u){i.e(u)}finally{i.f()}return n.join(`
`)}function Sd(){return jo(Array.from(Br.values()).map(function(e){return e.shape}))}function Rd(e){var n=Array.from(Br.entries());if(n.length===0)return`
float querySDF(int shapeId, vec2 uv, float size) {
  return length(uv) - size;
}
`;var t=n.map(function(r,a){var i=re(r,2),o=i[0],s=i[1].shape,l=Nr(s,"uv","size",function(u){var d=u.name.replace(/^a_/,"");return e!=null&&e.has(d)?"g_".concat(d):Ho(s,u)});return"    case ".concat(a,": return ").concat(l,"; // ").concat(o)}).join(`
`);return`
float querySDF(int shapeId, vec2 uv, float size) {
  switch (shapeId) {
`.concat(t,`
    default: return length(uv) - size;
  }
}
`)}function za(e){return jo(e)}function Pn(e){return K(new Map(e.flatMap(function(n){return n.uniforms}).map(function(n){return[n.name,n]})).values())}function Ad(e,n){if(e.length===0)return`
void queryNodeSDF(int shapeId, vec2 uv, float size) {
  context.sdf = length(uv) - size;
  context.inradiusFactor = 1.0;
}
`;var t=function(c){return Nr(c,"uv","size")},r=function(c){var g,h;return(g=c.inradiusFactorGLSL)!==null&&g!==void 0?g:ye((h=c.inradiusFactor)!==null&&h!==void 0?h:1)};if(e.length===1){var a=e[0];return`
void queryNodeSDF(int shapeId, vec2 uv, float size) {
  context.sdf = `.concat(t(a),`;
  context.inradiusFactor = `).concat(r(a),`;
}
`)}var i=e.map(function(d,c){return"    case ".concat(c,": // ").concat(d.name,`
      context.sdf = `).concat(t(d),`;
      context.inradiusFactor = `).concat(r(d),`;
      break;`)}).join(`
`),o=e[0],s="",l="shapeId";if(n&&n.length>1){var u=n.map(function(d,c){return"    case ".concat(d,": return ").concat(c,"; // ").concat(e[c].name)}).join(`
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
      context.sdf = `).concat(t(o),`;
      context.inradiusFactor = `).concat(r(o),`;
  }
}
`)}var ln={right:0,left:1,above:2,below:3,over:4},un=5,Ba=3,$o=`
float matrixScaleX = length(vec2(u_matrix[0][0], u_matrix[1][0]));
float nodeRadiusGraphSpace = nodeSize * u_correctionRatio / u_sizeRatio * 2.0;
float nodeRadiusNDC = nodeRadiusGraphSpace * matrixScaleX;
float nodeRadiusPixels = nodeRadiusNDC * u_resolution.x / 2.0;
`,Dd=`
mat2 rotate2D(float angle) {
  float c = cos(angle);
  float s = sin(angle);
  return mat2(c, -s, s, c);
}
`,Vo=`
vec2 getLabelDirection(float positionMode) {
  if (positionMode < 0.5) return vec2(1.0, 0.0);   // Right
  if (positionMode < 1.5) return vec2(-1.0, 0.0);  // Left
  if (positionMode < 2.5) return vec2(0.0, -1.0);  // Above
  if (positionMode < 3.5) return vec2(0.0, 1.0);   // Below
  return vec2(0.0);                                 // Over (centered)
}
`,wd=`
float sdfBox(vec2 p, vec2 halfSize) {
  vec2 d = abs(p) - halfSize;
  return length(max(d, 0.0)) + min(max(d.x, d.y), 0.0);
}
`,kd=`
float sdfRotatedBox(vec2 p, vec2 halfSize, float angle) {
  float c = cos(-angle);
  float s = sin(-angle);
  vec2 rotatedP = mat2(c, -s, s, c) * p;
  return sdfBox(rotatedP, halfSize);
}
`,Cd=`
float sdfRoundedBox(vec2 p, vec2 halfSize, float radius) {
  vec2 d = abs(p) - halfSize + radius;
  return length(max(d, 0.0)) + min(max(d.x, d.y), 0.0) - radius;
}
`,Fd=`
float sdfRoundedRotatedBox(vec2 p, vec2 halfSize, float angle, float radius) {
  float c = cos(-angle);
  float s = sin(-angle);
  vec2 rotatedP = mat2(c, -s, s, c) * p;
  return sdfRoundedBox(rotatedP, halfSize, radius);
}
`,qo=`
vec2 labelBoxCenter(float positionMode, float labelStart, vec2 halfSize, float textHalfY) {
  if (positionMode < 0.5) return vec2(labelStart + halfSize.x, 0.0);    // right
  if (positionMode < 1.5) return vec2(-(labelStart + halfSize.x), 0.0); // left
  if (positionMode < 2.5) return vec2(0.0, -(labelStart + textHalfY));  // above
  if (positionMode < 3.5) return vec2(0.0, labelStart + textHalfY);     // below
  return vec2(0.0);                                                     // over
}
`,Na=2,ct=`
vec4 readNodeData(sampler2D nodeDataTexture, int nodeDataTextureWidth, int nodeIndex) {
  int t = nodeIndex * `.concat(Na,`;
  ivec2 coord = ivec2(t % nodeDataTextureWidth, t / nodeDataTextureWidth);
  return texelFetch(nodeDataTexture, coord, 0);
}
`),It=`
vec4 readNodeFlags(sampler2D nodeDataTexture, int nodeDataTextureWidth, int nodeIndex) {
  int t = nodeIndex * `.concat(Na,` + 1;
  ivec2 coord = ivec2(t % nodeDataTextureWidth, t / nodeDataTextureWidth);
  return texelFetch(nodeDataTexture, coord, 0);
}
`),Ld=`
vec4 readNodeColor(sampler2D nodeDataTexture, int nodeDataTextureWidth, int nodeIndex) {
  int t = nodeIndex * `.concat(Na,` + 1;
  ivec2 coord = ivec2(t % nodeDataTextureWidth, t / nodeDataTextureWidth);
  vec4 texel = texelFetch(nodeDataTexture, coord, 0);
  float r = floor(texel.b / 65536.0);
  float g = floor(mod(texel.b, 65536.0) / 256.0);
  float b = mod(texel.b, 256.0);
  return vec4(r / 255.0, g / 255.0, b / 255.0, texel.a);
}
`),cn=`
vec4 readFrameTexel(sampler2D frameTexture, int frameTextureWidth, int index) {
  ivec2 coord = ivec2(index % frameTextureWidth, index / frameTextureWidth);
  return texelFetch(frameTexture, coord, 0);
}
`;function Pd(e,n){var t=function(o){return Nr(o,"uv","size")};if(e.length===1)return{code:Lo(t(e[0])),multiShape:!1};var r=e.map(function(i,o){return"    case ".concat(n?n[o]:o,": return ").concat(t(i),";")}).join(`
`),a=`
float queryShapeSDF(int shapeId, vec2 uv, float size) {
  switch (shapeId) {
`.concat(r,`
    default: return `).concat(t(e[0]),`;
  }
}
int g_shapeId;
`).concat(Lo("queryShapeSDF(g_shapeId, uv, size)"),`
`);return{code:a,multiShape:!0}}function Lo(e){return`
float findEdgeDistance(vec2 direction, float size) {
  float lo = 0.0, hi = 2.0;
  for (int i = 0; i < 8; i++) {
    float mid = (lo + hi) * 0.5;
    vec2 uv = direction * mid;
    if (`.concat(e,` < 0.0) lo = mid; else hi = mid;
  }
  return (lo + hi) * 0.5;
}
`)}function Id(e,n){for(;!{}.hasOwnProperty.call(e,n)&&(e=Ct(e))!==null;);return e}function La(){return La=typeof Reflect<"u"&&Reflect.get?Reflect.get.bind():function(e,n,t){var r=Id(e,n);if(r){var a=Object.getOwnPropertyDescriptor(r,n);return a.get?a.get.call(arguments.length<3?e:t):a.value}},La.apply(null,arguments)}function Ee(e,n,t,r){var a=La(Ct(1&r?e.prototype:e),n,t);return 2&r&&typeof a=="function"?function(i){return a.apply(t,i)}:a}var Gd=1024,zd=1.5,Bd=4096,Xo=-1,Mr=(function(){function e(n,t){var r=arguments.length>2&&arguments[2]!==void 0?arguments[2]:Gd;ee(this,e),D(this,"texture",null),D(this,"dirty",!1),D(this,"dirtyRangeStart",1/0),D(this,"dirtyRangeEnd",-1),D(this,"indexMap",new Map),D(this,"freeIndices",[]),D(this,"nextIndex",0),this.gl=n,this.TEXELS_PER_ITEM=t,this.capacity=this.roundUpToPowerOfTwo(r);var a=this.computeTextureDimensions(this.capacity);this.textureWidth=a.width,this.textureHeight=a.height,this.data=new Float32Array(this.textureWidth*this.textureHeight*4),this.createTexture()}return te(e,[{key:"computeTextureDimensions",value:function(t){var r=t*this.TEXELS_PER_ITEM,a=Math.min(r,Bd),i=Math.ceil(r/a);return{width:a,height:i}}},{key:"roundUpToPowerOfTwo",value:function(t){return Math.pow(2,Math.ceil(Math.log2(Math.max(1,t))))}},{key:"createTexture",value:function(){var t=this.gl;this.texture=t.createTexture(),t.activeTexture(t.TEXTURE0),t.bindTexture(t.TEXTURE_2D,this.texture),t.texImage2D(t.TEXTURE_2D,0,t.RGBA32F,this.textureWidth,this.textureHeight,0,t.RGBA,t.FLOAT,this.data),t.texParameteri(t.TEXTURE_2D,t.TEXTURE_MIN_FILTER,t.NEAREST),t.texParameteri(t.TEXTURE_2D,t.TEXTURE_MAG_FILTER,t.NEAREST),t.texParameteri(t.TEXTURE_2D,t.TEXTURE_WRAP_S,t.CLAMP_TO_EDGE),t.texParameteri(t.TEXTURE_2D,t.TEXTURE_WRAP_T,t.CLAMP_TO_EDGE),t.bindTexture(t.TEXTURE_2D,null)}},{key:"resize",value:function(t){if(!(t<=this.capacity)){var r=this.roundUpToPowerOfTwo(Math.ceil(t*zd)),a=this.gl,i=this.computeTextureDimensions(r),o=new Float32Array(i.width*i.height*4);o.set(this.data),this.texture&&a.deleteTexture(this.texture),this.data=o,this.capacity=r,this.textureWidth=i.width,this.textureHeight=i.height,this.createTexture(),this.dirty=!0,this.dirtyRangeStart=0,this.dirtyRangeEnd=this.nextIndex}}},{key:"allocate",value:function(t){var r=this.indexMap.get(t);if(r!==void 0)return r;var a;return this.freeIndices.length>0?a=this.freeIndices.pop():(a=this.nextIndex++,a>=this.capacity&&this.resize(a+1)),this.indexMap.set(t,a),a}},{key:"free",value:function(t){var r=this.indexMap.get(t);if(r!==void 0){this.indexMap.delete(t),this.freeIndices.push(r);for(var a=r*this.TEXELS_PER_ITEM*4,i=0;i<this.TEXELS_PER_ITEM*4;i++)this.data[a+i]=0;this.markDirty(r)}}},{key:"getIndex",value:function(t){var r;return(r=this.indexMap.get(t))!==null&&r!==void 0?r:-1}},{key:"has",value:function(t){return this.indexMap.has(t)}},{key:"markDirty",value:function(t){this.dirty=!0,this.dirtyRangeStart=Math.min(this.dirtyRangeStart,t),this.dirtyRangeEnd=Math.max(this.dirtyRangeEnd,t+1)}},{key:"upload",value:function(){if(!(!this.dirty||!this.texture)){var t=this.gl,r=this.textureWidth;t.activeTexture(t.TEXTURE0),t.bindTexture(t.TEXTURE_2D,this.texture);var a=this.dirtyRangeStart*this.TEXELS_PER_ITEM,i=Math.min(this.dirtyRangeEnd*this.TEXELS_PER_ITEM,this.capacity*this.TEXELS_PER_ITEM);if(a<i)for(var o=Math.floor(a/r),s=Math.floor((i-1)/r),l=o;l<=s;l++){var u=l*r,d=Math.min(u+r,this.capacity*this.TEXELS_PER_ITEM),c=Math.max(a,u),g=Math.min(i,d);if(c<g){var h=c-u,f=g-c,y=this.data.subarray(c*4,g*4);t.texSubImage2D(t.TEXTURE_2D,0,h,l,f,1,t.RGBA,t.FLOAT,y)}}t.bindTexture(t.TEXTURE_2D,null),this.dirty=!1,this.dirtyRangeStart=1/0,this.dirtyRangeEnd=-1}}},{key:"bind",value:function(t){var r=this.gl;r.activeTexture(r.TEXTURE0+t),r.bindTexture(r.TEXTURE_2D,this.texture)}},{key:"getTexture",value:function(){return this.texture}},{key:"getCapacity",value:function(){return this.capacity}},{key:"getTextureWidth",value:function(){return this.textureWidth}},{key:"getTextureHeight",value:function(){return this.textureHeight}},{key:"getTexelsPerItem",value:function(){return this.TEXELS_PER_ITEM}},{key:"getCount",value:function(){return this.indexMap.size}},{key:"getHighWaterMark",value:function(){return this.nextIndex}},{key:"isDirty",value:function(){return this.dirty}},{key:"clear",value:function(){this.indexMap.clear(),this.freeIndices=[],this.nextIndex=0,this.data.fill(0),this.dirty=!0,this.dirtyRangeStart=0,this.dirtyRangeEnd=this.capacity}},{key:"restore",value:function(){this.createTexture(),this.dirty=!1,this.dirtyRangeStart=1/0,this.dirtyRangeEnd=-1}},{key:"kill",value:function(){this.texture&&(this.gl.deleteTexture(this.texture),this.texture=null),this.indexMap.clear(),this.freeIndices=[]}}])})();function Oe(e){var n={},t={},r=0,a=U(e),i;try{for(a.s();!(i=a.n()).done;){var o=i.value,s=U(o.attributes),l;try{for(s.s();!(l=s.n()).done;){var u=l.value,d=u.name.replace(/^a_/,"");d in n||(n[d]=r,t[d]=u,r+=u.size)}}catch(c){s.e(c)}finally{s.f()}}}catch(c){a.e(c)}finally{a.f()}return{floatsPerItem:r,texelsPerItem:Math.max(1,Math.ceil(r/4)),offsets:n,specs:t}}var Wr=(function(e){function n(t,r,a){var i;return ee(this,n),i=ge(this,n,[t,r.texelsPerItem,a]),i.floatsPerItem=r.floatsPerItem,i}return pe(n,e),te(n,[{key:"updateAllAttributes",value:function(r,a){var i=this.indexMap.get(r);i===void 0&&(i=this.allocate(r));for(var o=i*this.TEXELS_PER_ITEM*4,s=Math.min(a.length,this.floatsPerItem),l=0;l<s;l++)this.data[o+l]=a[l];this.markDirty(i)}},{key:"updateAllAttributesAtRow",value:function(r,a){r>=this.capacity&&this.resize(r+1);for(var i=r*this.TEXELS_PER_ITEM*4,o=Math.min(a.length,this.floatsPerItem),s=0;s<o;s++)this.data[i+s]=a[s];this.markDirty(r)}}])})(Mr);function Ur(e,n,t){for(var r=[],a=new Set,i=0;i<e.length;i++){var o=e[i],s=U(o.attributes),l;try{for(s.s();!(l=s.n()).done;){var u=l.value,d=u.name.replace(/^a_/,"");if(!a.has(d)){a.add(d);var c=n.offsets[d];if(c!==void 0){var g=t?.get(i);r.push({sourceKey:u.source||d,packedOffset:c,size:u.size,isColor:u.size===4&&!!u.normalized,defaultNum:typeof u.defaultValue=="number"?u.defaultValue:u.defaultValue===!0?1:0,defaultColor:typeof u.defaultValue=="string"?u.defaultValue:"",sourceIndex:i,hasLifecycleHook:!!(g!=null&&g.getAttributeData)})}}}}catch(h){s.e(h)}finally{s.f()}}return r}function Or(e,n,t,r,a,i){t.fill(0);for(var o=0,s=e.length;o<s;o++){var l=e[o],u=l.packedOffset,d=void 0;if(l.hasLifecycleHook){var c=a.get(l.sourceIndex-i);d=c.getAttributeData(n,l.sourceKey),d===null&&(d=n[l.sourceKey])}else d=n[l.sourceKey];if(l.isColor){var g=typeof d=="string"?d:l.defaultColor||r,h=pt(g),f=re(h,4),y=f[0],v=f[1],p=f[2],m=f[3];t[u]=y/255,t[u+1]=v/255,t[u+2]=p/255,t[u+3]=m/255}else if(l.size===1)t[u]=typeof d=="number"?d:typeof d=="boolean"?d?1:0:l.defaultNum;else{var b=Array.isArray(d)?d:null;if(b)for(var x=0;x<l.size;x++){var _;t[u+x]=(_=b[x])!==null&&_!==void 0?_:0}}}}var Lr=["r","g","b","a"];function sn(e,n){var t=e.offsets,r=e.specs,a=e.texelsPerItem,i=e.floatsPerItem,o=Object.keys(t);if(o.length===0||i===0)return{fetchCode:"",varyingAssignments:""};var s=n.varPrefix,l=n.baseTexelExpr,u=n.textureWidthUniform,d=n.textureSamplerUniform,c=n.outputPrefix,g=c===void 0?"v_":c,h=[];h.push("  int ".concat(s,"BaseTexel = ").concat(l,";")),h.push("");for(var f=0;f<a;f++)h.push("  ivec2 ".concat(s,"Coord").concat(f," = ivec2((").concat(s,"BaseTexel + ").concat(f,") % ").concat(u,", (").concat(s,"BaseTexel + ").concat(f,") / ").concat(u,");")),h.push("  vec4 ".concat(s,"Texel").concat(f," = texelFetch(").concat(d,", ").concat(s,"Coord").concat(f,", 0);"));h.push("");for(var y=[],v=function(){var x=m[p],_=r[x],T=t[x],R=Math.floor(T/4),A=T%4,S="".concat(s,"Texel").concat(R),k="".concat(s,"Texel").concat(R+1),L="".concat(s,"Fetched_").concat(x);if(_.size===1)h.push("  float ".concat(L," = ").concat(S,".").concat(Lr[A],";"));else if(_.size===4&&A===0)h.push("  vec4 ".concat(L," = ").concat(S,";"));else{var C=A+_.size;if(C<=4){var z="vec".concat(_.size),F=Lr.slice(A,C).join("");h.push("  ".concat(z," ").concat(L," = ").concat(S,".").concat(F,";"))}else{var P=_.size===4?"vec4":"vec".concat(_.size),N=Lr.slice(A).map(function(I){return"".concat(S,".").concat(I)}),G=Lr.slice(0,C-4).map(function(I){return"".concat(k,".").concat(I)}),E=[].concat(K(N),K(G)).join(", ");h.push("  ".concat(P," ").concat(L," = ").concat(P,"(").concat(E,");"))}}y.push("  ".concat(g).concat(x," = ").concat(L,";"))},p=0,m=o;p<m.length;p++)v();return{fetchCode:h.join(`
`),varyingAssignments:y.join(`
`)}}function Ma(e,n,t){if(!(!n||t.type==="sampler2D"))switch(t.type){case"float":e.uniform1f(n,t.value);break;case"int":case"bool":e.uniform1i(n,t.value);break;case"vec2":e.uniform2fv(n,t.value);break;case"vec3":e.uniform3fv(n,t.value);break;case"vec4":e.uniform4fv(n,t.value);break;case"mat3":e.uniformMatrix3fv(n,!1,t.value);break;case"mat4":e.uniformMatrix4fv(n,!1,t.value);break}}var Nd=D(D(D(D(D(D(D(D({},WebGL2RenderingContext.BOOL,1),WebGL2RenderingContext.BYTE,1),WebGL2RenderingContext.UNSIGNED_BYTE,1),WebGL2RenderingContext.SHORT,2),WebGL2RenderingContext.UNSIGNED_SHORT,2),WebGL2RenderingContext.INT,4),WebGL2RenderingContext.UNSIGNED_INT,4),WebGL2RenderingContext.FLOAT,4);function Pr(e){var n=e.match(/^(#version[^\n]*\n)/);return n?n[1]+`#define PICKING_MODE
`+e.slice(n[1].length):`#define PICKING_MODE
`+e}var Gt=(function(){function e(n,t,r){ee(this,e),D(this,"floats",new Float32Array),D(this,"ints",new Uint32Array),D(this,"constantArray",new Float32Array),D(this,"capacity",0),D(this,"verticesCount",0),D(this,"bufferGeneration",0),D(this,"uploadedGeneration",new Map),D(this,"constantBufferGeneration",0),D(this,"uploadedConstantGeneration",new Map),D(this,"renderOffset",0),D(this,"renderCount",-1),D(this,"debugStats",{drawCalls:0,verticesDrawn:0,bufferUploadBytes:0}),D(this,"shadersLogged",!1),D(this,"pickProgram",null);var a=this.getDefinition();if(this.VERTICES=a.VERTICES,this.VERTEX_SHADER_SOURCE=a.VERTEX_SHADER_SOURCE,this.FRAGMENT_SHADER_SOURCE=a.FRAGMENT_SHADER_SOURCE,this.UNIFORMS=a.UNIFORMS,this.ATTRIBUTES=a.ATTRIBUTES,this.METHOD=a.METHOD,this.CONSTANT_ATTRIBUTES="CONSTANT_ATTRIBUTES"in a?a.CONSTANT_ATTRIBUTES:[],this.CONSTANT_DATA="CONSTANT_DATA"in a?a.CONSTANT_DATA:[],this.isInstanced="CONSTANT_ATTRIBUTES"in a,this.ATTRIBUTES_ITEMS_COUNT=Fa(this.ATTRIBUTES),this.STRIDE=this.VERTICES*this.ATTRIBUTES_ITEMS_COUNT,this.renderer=r,this.normalProgram=this.getProgramInfo("normal",n,a.VERTEX_SHADER_SOURCE,a.FRAGMENT_SHADER_SOURCE,null),this.pickProgram=this.getProgramInfo("pick",n,Pr(a.VERTEX_SHADER_SOURCE),Pr(a.FRAGMENT_SHADER_SOURCE),null),this.isInstanced){var i=Fa(this.CONSTANT_ATTRIBUTES);if(this.CONSTANT_DATA.length!==this.VERTICES)throw new Error("Program: error while getting constant data (expected ".concat(this.VERTICES," items, received ").concat(this.CONSTANT_DATA.length," instead)"));this.constantArray=new Float32Array(this.CONSTANT_DATA.length*i);for(var o=0;o<this.CONSTANT_DATA.length;o++){var s=this.CONSTANT_DATA[o];if(s.length!==i)throw new Error("Program: error while getting constant data (one vector has ".concat(s.length," items instead of ").concat(i,")"));for(var l=0;l<s.length;l++)this.constantArray[o*i+l]=s[l]}this.STRIDE=this.ATTRIBUTES_ITEMS_COUNT}}return te(e,[{key:"kill",value:function(){Fo(this.normalProgram),this.pickProgram&&Fo(this.pickProgram)}},{key:"resetDebugStats",value:function(){this.debugStats.drawCalls=0,this.debugStats.verticesDrawn=0,this.debugStats.bufferUploadBytes=0}},{key:"getProgramInfo",value:function(t,r,a,i,o){var s=r.createBuffer();if(s===null)throw new Error("Program: error while creating the WebGL buffer.");var l=Pa(r,a),u=Ia(r,i),d=Ga(r,[l,u]),c={};this.UNIFORMS.forEach(function(f){var y=r.getUniformLocation(d,f);y&&(c[f]=y)});var g={};this.ATTRIBUTES.forEach(function(f){g[f.name]=r.getAttribLocation(d,f.name)});var h;if(this.isInstanced&&(this.CONSTANT_ATTRIBUTES.forEach(function(f){g[f.name]=r.getAttribLocation(d,f.name)}),h=r.createBuffer(),h===null))throw new Error("Program: error while creating the WebGL constant buffer.");return{name:t,program:d,gl:r,frameBuffer:o,buffer:s,constantBuffer:h||{},uniformLocations:c,attributeLocations:g,isPicking:t==="pick",vertexShader:l,fragmentShader:u}}},{key:"bindProgram",value:function(t){var r=this,a=0,i=t.gl,o=t.buffer;if(this.isInstanced){if(i.bindBuffer(i.ARRAY_BUFFER,t.constantBuffer),a=0,this.CONSTANT_ATTRIBUTES.forEach(function(d){return a+=r.bindAttribute(d,t,a,!1)}),this.uploadedConstantGeneration.get(t.constantBuffer)!==this.constantBufferGeneration){var l;i.bufferData(i.ARRAY_BUFFER,this.constantArray,i.STATIC_DRAW),this.uploadedConstantGeneration.set(t.constantBuffer,this.constantBufferGeneration),(l=this.renderer)!==null&&l!==void 0&&l.getSetting("DEBUG_logRenderStats")&&(this.debugStats.bufferUploadBytes+=this.constantArray.byteLength)}if(i.bindBuffer(i.ARRAY_BUFFER,t.buffer),a=this.renderOffset*this.ATTRIBUTES_ITEMS_COUNT*Float32Array.BYTES_PER_ELEMENT,this.ATTRIBUTES.forEach(function(d){return a+=r.bindAttribute(d,t,a,!0)}),this.uploadedGeneration.get(o)!==this.bufferGeneration){var u;i.bufferData(i.ARRAY_BUFFER,this.floats,i.DYNAMIC_DRAW),this.uploadedGeneration.set(o,this.bufferGeneration),(u=this.renderer)!==null&&u!==void 0&&u.getSetting("DEBUG_logRenderStats")&&(this.debugStats.bufferUploadBytes+=this.floats.byteLength)}}else if(i.bindBuffer(i.ARRAY_BUFFER,o),a=0,this.ATTRIBUTES.forEach(function(d){return a+=r.bindAttribute(d,t,a)}),this.uploadedGeneration.get(o)!==this.bufferGeneration){var s;i.bufferData(i.ARRAY_BUFFER,this.floats,i.DYNAMIC_DRAW),this.uploadedGeneration.set(o,this.bufferGeneration),(s=this.renderer)!==null&&s!==void 0&&s.getSetting("DEBUG_logRenderStats")&&(this.debugStats.bufferUploadBytes+=this.floats.byteLength)}i.bindBuffer(i.ARRAY_BUFFER,null)}},{key:"unbindProgram",value:function(t){var r=this;this.isInstanced?(this.CONSTANT_ATTRIBUTES.forEach(function(a){return r.unbindAttribute(a,t,!1)}),this.ATTRIBUTES.forEach(function(a){return r.unbindAttribute(a,t,!0)})):this.ATTRIBUTES.forEach(function(a){return r.unbindAttribute(a,t)})}},{key:"bindAttribute",value:function(t,r,a,i){var o=Nd[t.type];if(typeof o!="number")throw new Error('Program.bind: yet unsupported attribute type "'.concat(t.type,'"'));var s=r.attributeLocations[t.name],l=r.gl;if(s!==-1){l.enableVertexAttribArray(s);var u=this.isInstanced?(i?this.ATTRIBUTES_ITEMS_COUNT:Fa(this.CONSTANT_ATTRIBUTES))*Float32Array.BYTES_PER_ELEMENT:this.ATTRIBUTES_ITEMS_COUNT*Float32Array.BYTES_PER_ELEMENT;l.vertexAttribPointer(s,t.size,t.type,t.normalized||!1,u,a),this.isInstanced&&i&&l.vertexAttribDivisor(s,1)}return t.size*o}},{key:"unbindAttribute",value:function(t,r,a){var i=r.attributeLocations[t.name],o=r.gl;i!==-1&&(o.disableVertexAttribArray(i),this.isInstanced&&a&&o.vertexAttribDivisor(i,0))}},{key:"reallocate",value:function(t){t!==this.capacity&&(this.capacity=t,this.verticesCount=this.VERTICES*t,this.floats=new Float32Array(this.isInstanced?this.capacity*this.ATTRIBUTES_ITEMS_COUNT:this.verticesCount*this.ATTRIBUTES_ITEMS_COUNT),this.ints=new Uint32Array(this.floats.buffer),this.invalidateBuffers())}},{key:"invalidateBuffers",value:function(){this.bufferGeneration++,this.constantBufferGeneration++}},{key:"hasNothingToRender",value:function(){return this.verticesCount===0}},{key:"setTypedUniform",value:function(t,r){var a;Ma(r.gl,(a=r.uniformLocations[t.name])!==null&&a!==void 0?a:null,t)}},{key:"renderProgram",value:function(t,r){var a=r.gl,i=r.program,o=r.isPicking;o?a.disable(a.BLEND):a.enable(a.BLEND),a.useProgram(i),this.setUniforms(t,r),this.drawWebGL(this.METHOD,r)}},{key:"render",value:function(t,r,a){var i;if(!this.shadersLogged&&(i=this.renderer)!==null&&i!==void 0&&i.getSetting("DEBUG_logShaders")&&(this.shadersLogged=!0,console.log("[sigma] DEBUG_logShaders: ".concat(this.constructor.name),{normal:{vertexShaderSource:this.VERTEX_SHADER_SOURCE,fragmentShaderSource:this.FRAGMENT_SHADER_SOURCE},pick:{vertexShaderSource:Pr(this.VERTEX_SHADER_SOURCE),fragmentShaderSource:Pr(this.FRAGMENT_SHADER_SOURCE)}})),!this.hasNothingToRender()){this.renderOffset=r??0,this.renderCount=a??-1;var o=this.normalProgram.gl;if(o.bindFramebuffer(o.FRAMEBUFFER,null),o.viewport(0,0,t.width*t.pixelRatio,t.height*t.pixelRatio),this.bindProgram(this.normalProgram),this.renderProgram(t,this.normalProgram),this.unbindProgram(this.normalProgram),this.pickProgram&&t.pickingFrameBuffer){var s=Math.ceil(t.width*t.pixelRatio/t.downSizingRatio),l=Math.ceil(t.height*t.pixelRatio/t.downSizingRatio);o.bindFramebuffer(o.FRAMEBUFFER,t.pickingFrameBuffer),o.viewport(0,0,s,l),this.bindProgram(this.pickProgram),this.renderProgram(t,this.pickProgram),this.unbindProgram(this.pickProgram),o.bindFramebuffer(o.FRAMEBUFFER,null),o.viewport(0,0,t.width*t.pixelRatio,t.height*t.pixelRatio)}}}},{key:"drawWebGL",value:function(t,r){var a,i=r.gl,o=this.renderCount>=0?this.renderCount:this.capacity;(a=this.renderer)!==null&&a!==void 0&&a.getSetting("DEBUG_logRenderStats")&&(this.debugStats.drawCalls++,this.debugStats.verticesDrawn+=o*this.VERTICES),this.isInstanced?i.drawArraysInstanced(t,0,this.VERTICES,o):i.drawArrays(t,this.renderOffset*this.VERTICES,o*this.VERTICES)}}])})(),yt=2,In=7,Yo={below:0,above:1,left:2,right:3};function Md(){var e=`#version 300 es
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

`.concat(ct,`
`).concat(It,`
`).concat(cn,`
`).concat(qo,`
`).concat(Dd,`

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
  float gap = `).concat(yt.toFixed(1),` * zoomScale * u_pixelRatio;
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
`);return e}var Wd=`#version 300 es
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
`;function Ud(e,n,t,r){var a,i,o=r.label,s=o===void 0?{}:o,l=(a=s.margin)!==null&&a!==void 0?a:un,u=(i=s.zoomToLabelSizeRatioFunction)!==null&&i!==void 0?i:function(){return 1},d=Md(),c=Wd,g=(function(h){function f(){var y;ee(this,f);for(var v=arguments.length,p=new Array(v),m=0;m<v;m++)p[m]=arguments[m];return y=ge(this,f,[].concat(p)),D(y,"totalCount",0),D(y,"bufferCapacity",0),y}return pe(f,h),te(f,[{key:"getDefinition",value:function(){var v=WebGL2RenderingContext,p=v.FLOAT,m=v.TRIANGLE_STRIP;return{VERTICES:4,VERTEX_SHADER_SOURCE:d,FRAGMENT_SHADER_SOURCE:c,METHOD:m,UNIFORMS:["u_matrix","u_resolution","u_labelPixelSnapping","u_sizeRatio","u_correctionRatio","u_cameraAngle","u_pixelRatio","u_labelMargin","u_zoomLabelSizeRatio","u_nodeDataTexture","u_nodeDataTextureWidth","u_nodeFrameTexture","u_nodeFrameTextureWidth","u_atlasTexture"],ATTRIBUTES:[{name:"a_nodeIndex",size:1,type:p},{name:"a_atlasRect",size:4,type:p},{name:"a_attachmentSize",size:2,type:p},{name:"a_positionMode",size:1,type:p},{name:"a_attachmentPlacement",size:1,type:p},{name:"a_labelWidth",size:1,type:p},{name:"a_labelHeight",size:1,type:p},{name:"a_textHeight",size:1,type:p},{name:"a_labelAngle",size:1,type:p}],CONSTANT_ATTRIBUTES:[{name:"a_quadCorner",size:2,type:p}],CONSTANT_DATA:[[-1,-1],[1,-1],[-1,1],[1,1]]}}},{key:"processAttachment",value:function(v,p){var m=this.floats,b=v*this.STRIDE;m[b++]=p.nodeIndex,m[b++]=p.atlasX,m[b++]=p.atlasY,m[b++]=p.atlasW,m[b++]=p.atlasH,m[b++]=p.attachWidth,m[b++]=p.attachHeight,m[b++]=p.positionMode,m[b++]=p.attachmentPlacement,m[b++]=p.labelWidth,m[b++]=p.labelHeight,m[b++]=p.textHeight,m[b++]=p.labelAngle}},{key:"setUniforms",value:function(v,p){var m=p.gl,b=p.uniformLocations;m.uniformMatrix3fv(b.u_matrix,!1,v.matrix),m.uniform2f(b.u_resolution,v.width*v.pixelRatio,v.height*v.pixelRatio),m.uniform1f(b.u_labelPixelSnapping,v.labelPixelSnapping),m.uniform1f(b.u_sizeRatio,v.sizeRatio),m.uniform1f(b.u_correctionRatio,v.correctionRatio),m.uniform1f(b.u_cameraAngle,v.cameraAngle),m.uniform1f(b.u_pixelRatio,v.pixelRatio),m.uniform1f(b.u_labelMargin,f.labelMargin),m.uniform1f(b.u_zoomLabelSizeRatio,1/u(v.zoomRatio)),m.uniform1i(b.u_nodeDataTexture,v.nodeDataTextureUnit),m.uniform1i(b.u_nodeDataTextureWidth,v.nodeDataTextureWidth),m.uniform1i(b.u_nodeFrameTexture,v.nodeFrameTextureUnit),m.uniform1i(b.u_nodeFrameTextureWidth,v.nodeFrameTextureWidth),m.uniform1i(b.u_atlasTexture,In)}},{key:"reallocateAttachments",value:function(v){this.totalCount=v,v>this.bufferCapacity&&(this.bufferCapacity=Math.max(v,Math.ceil(this.bufferCapacity*1.5)||10),Ee(f,"reallocate",this,3)([this.bufferCapacity]))}},{key:"hasNothingToRender",value:function(){return this.totalCount===0}},{key:"drawWebGL",value:function(v,p){var m=p.gl;this.totalCount!==0&&m.drawArraysInstanced(v,0,this.VERTICES,this.totalCount)}}])})(Gt);return D(g,"labelMargin",l),new g(e,n,t)}var mt=5;function Hr(e,n){return[].concat(K(n),K(e.map(function(t){var r;return{attributes:(r=t.attributes)!==null&&r!==void 0?r:[]}})))}function dn(e,n){var t=Oe(Hr(e,n)),r=new Set(e.flatMap(function(u){var d;return(d=u.attributes)!==null&&d!==void 0?d:[]}).map(function(u){return u.name.replace(/^a_/,"")})),a={},i={},o=U(r),s;try{for(o.s();!(s=o.n()).done;){var l=s.value;a[l]=t.offsets[l],i[l]=t.specs[l]}}catch(u){o.e(u)}finally{o.f()}return O(O({},t),{},{offsets:a,specs:i})}function Od(e,n){var t=new Set(["u_matrix","u_sizeRatio","u_correctionRatio","u_cameraAngle","u_pickingPadding","u_nodeDataTexture","u_layerAttributeTexture"]),r=new Set,a=e.flatMap(function(h){return h.uniforms}).filter(function(h){return t.has(h.name)||r.has(h.name)?!1:(r.add(h.name),!0)}).map(function(h){return"uniform ".concat(h.type," ").concat(h.name,";")}).join(`
`),i=n.flatMap(function(h){return h.uniforms}).filter(function(h){return t.has(h.name)||r.has(h.name)?!1:(r.add(h.name),!0)}).map(function(h){return"uniform ".concat(h.type," ").concat(h.name,";")}).join(`
`),o=Hr(e,n),s=new Set,l=o.flatMap(function(h){return h.attributes}).filter(function(h){var f=h.name.replace(/^a_/,"");return s.has(f)?!1:(s.add(f),!0)}).map(function(h){var f=h.name.replace(/^a_/,""),y=h.size===1?"float":"vec".concat(h.size);return"out ".concat(y," v_").concat(f,";")}).join(`
`),u=sn(Oe(o),{varPrefix:"layer",baseTexelExpr:"nodeIdx * u_layerAttributeTexelsPerNode",textureWidthUniform:"u_layerAttributeTextureWidth",textureSamplerUniform:"u_layerAttributeTexture"}),d=u.fetchCode,c=u.varyingAssignments,g=`#version 300 es

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
`).concat(ct,`
`).concat(It,`

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

`).concat(d,`

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
`).concat(c,`
}
`);return g}function Hd(e,n,t){var r=arguments.length>3&&arguments[3]!==void 0?arguments[3]:!0,a=n.map(function(f,y){var v="layer_".concat(f.name),p=[].concat(K(f.attributes.map(function(m){return"v_".concat(m.name.replace(/^a_/,""))})),K(f.uniforms.map(function(m){return m.name}))).join(", ");return"  // Layer ".concat(y+1,": ").concat(f.name,`
  color = blendOver(color, `).concat(v,"(").concat(p,"));")}).join(`

`),i=new Set(["u_correctionRatio"]),o=new Set,s=e.flatMap(function(f){return f.uniforms}).filter(function(f){return i.has(f.name)||o.has(f.name)?!1:(o.add(f.name),!0)}).map(function(f){return"uniform ".concat(f.type," ").concat(f.name,";")}).join(`
`),l=n.flatMap(function(f){return f.uniforms}).filter(function(f){return i.has(f.name)||o.has(f.name)?!1:(o.add(f.name),!0)}).map(function(f){return"uniform ".concat(f.type," ").concat(f.name,";")}).join(`
`),u=new Set,d=Hr(e,n).flatMap(function(f){return f.attributes}).filter(function(f){var y=f.name.replace(/^a_/,"");return u.has(y)?!1:(u.add(y),!0)}).map(function(f){var y=f.name.replace(/^a_/,""),v=f.size===1?"float":"vec".concat(f.size);return"in ".concat(v," v_").concat(y,";")}).join(`
`),c=za(e),g=Ad(e,t),h=`#version 300 es
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
`).concat(d,`

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
`).concat(c,`

// Shape selector function (sets context.sdf and context.inradiusFactor)
`).concat(g,`

// Layer functions
`).concat(n.map(function(f){return f.glsl}).join(`

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
`);return h}function jd(e,n){var t=new Set;return t.add("u_matrix"),t.add("u_sizeRatio"),t.add("u_correctionRatio"),t.add("u_cameraAngle"),t.add("u_pickingPadding"),t.add("u_nodeDataTexture"),t.add("u_nodeDataTextureWidth"),t.add("u_layerAttributeTexture"),t.add("u_layerAttributeTextureWidth"),t.add("u_layerAttributeTexelsPerNode"),e.forEach(function(r){r.uniforms.forEach(function(a){return t.add(a.name)})}),n.forEach(function(r){r.uniforms.forEach(function(a){return t.add(a.name)})}),Array.from(t)}function $d(e){var n=WebGL2RenderingContext,t=n.UNSIGNED_BYTE,r=n.FLOAT;return[{name:"a_nodeIndex",size:1,type:r},{name:"a_id",size:4,type:t,normalized:!0},{name:"a_opacity",size:1,type:r}]}function Po(e){var n=e.shapes,t=e.layers,r=e.shapeGlobalIds,a=e.antialias,i=a===void 0?!0:a;return{vertexShader:Od(n,t),fragmentShader:Hd(n,t,r,i),uniforms:jd(n,t),attributes:$d()}}function Jo(e,n,t){var r=dn(e,n),a=r.specs;return Object.keys(a).map(function(i){return"flat ".concat(t," ").concat(a[i].size===1?"float":"vec".concat(a[i].size)," v_").concat(i,";")}).join(`
`)}function Vd(e){var n,t,r=e.shapes,a=e.layers,i=e.shapeGlobalIds,o=sn(dn(r,a),{varPrefix:"nodeAttr",baseTexelExpr:"nodeIdx * u_layerAttributeTexelsPerNode",textureWidthUniform:"u_layerAttributeTextureWidth",textureSamplerUniform:"u_layerAttributeTexture"}),s=r.length===1?"float inradiusFactor = ".concat(ye((n=r[0].inradiusFactor)!==null&&n!==void 0?n:1),";"):"float inradiusFactor = ".concat(ye((t=r[0].inradiusFactor)!==null&&t!==void 0?t:1),`;
  switch (int(shapeId)) {
`).concat(r.map(function(u,d){var c,g=i?i[d]:d;return"    case ".concat(g,": inradiusFactor = ").concat(ye((c=u.inradiusFactor)!==null&&c!==void 0?c:1),"; break;")}).join(`
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
`.concat(Jo(r,a,"out"),`

`).concat(ct,`
`).concat(It,`
`).concat(cn,`

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

  `).concat($o,`
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
`);return l}function qd(e){var n=e.shapes,t=e.layers,r=e.shapeGlobalIds,a=za(n),i=Pn(n).map(function(g){return"uniform ".concat(g.type," ").concat(g.name,";")}).join(`
`),o=function(h){return Nr(h,"nodeUV","1.0")},s;if(n.length===1)s="float nodeSdfNormalized = ".concat(o(n[0]),";");else{var l=n.map(function(g,h){var f=r?r[h]:h;return"    case ".concat(f,": nodeSdfNormalized = ").concat(o(g),"; break;")}).join(`
`),u=o(n[0]);s=`float nodeSdfNormalized;
  int shapeId = int(v_shapeId);
  switch (shapeId) {
`.concat(l,`
    default: nodeSdfNormalized = `).concat(u,`;
  }`)}var d=`float effectiveRadius = max(enlargedRadius - cornerRadius, 0.01);
  float ca = u_cameraAngle * v_nodeRotation;
  float ca_c = cos(ca);
  float ca_s = sin(ca);
  vec2 rotatedScreenUV = mat2(ca_c, -ca_s, ca_s, ca_c) * screenUV;
  vec2 nodeUV = vec2(rotatedScreenUV.x, -rotatedScreenUV.y) / effectiveRadius;`,c=`#version 300 es
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
`.concat(Jo(n,t,"in"),`

uniform float u_cameraAngle;
`).concat(i,`

layout(location = 0) out vec4 fragColor;
layout(location = 1) out vec4 fragPicking;

`).concat(a,`
`).concat(wd,`
`).concat(kd,`
`).concat(Cd,`
`).concat(Fd,`

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
  `).concat(d,`

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
`);return c}function Xd(e){var n=["u_matrix","u_sizeRatio","u_correctionRatio","u_cameraAngle","u_resolution","u_pixelRatio","u_labelMargin","u_zoomLabelSizeRatio","u_labelPixelSnapping","u_nodeDataTexture","u_nodeDataTextureWidth","u_nodeFrameTexture","u_nodeFrameTextureWidth","u_layerAttributeTexture","u_layerAttributeTextureWidth","u_layerAttributeTexelsPerNode"],t=U(Pn(e)),r;try{for(t.s();!(r=t.n()).done;){var a=r.value;n.includes(a.name)||n.push(a.name)}}catch(i){t.e(i)}finally{t.f()}return n}function Yd(e){return{vertexShader:Vd(e),fragmentShader:qd(e),uniforms:Xd(e.shapes)}}function Jd(e,n,t,r){var a,i,o=r.label,s=o===void 0?{}:o,l=r.shapes,u=r.layers,d=r.getAttributeTexture,c=r.shapeGlobalIds;if(l.length===0)throw new Error("createBackdropProgram: at least one shape must be provided in 'shapes'");var g=(a=s.margin)!==null&&a!==void 0?a:5,h=(i=s.zoomToLabelSizeRatioFunction)!==null&&i!==void 0?i:function(){return 1},f={shapes:l,layers:u,shapeGlobalIds:c},y=Yd(f),v=(function(p){function m(){var b;ee(this,m);for(var x=arguments.length,_=new Array(x),T=0;T<x;T++)_[T]=arguments[T];return b=ge(this,m,[].concat(_)),D(b,"totalBackdropCount",0),D(b,"bufferCapacity",0),b}return pe(m,p),te(m,[{key:"getDefinition",value:function(){var x=WebGL2RenderingContext,_=x.FLOAT,T=x.TRIANGLE_STRIP;return{VERTICES:4,VERTEX_SHADER_SOURCE:y.vertexShader,FRAGMENT_SHADER_SOURCE:y.fragmentShader,METHOD:T,UNIFORMS:y.uniforms,ATTRIBUTES:[{name:"a_nodeIndex",size:1,type:_},{name:"a_labelWidth",size:1,type:_},{name:"a_labelHeight",size:1,type:_},{name:"a_textHeight",size:1,type:_},{name:"a_positionMode",size:1,type:_},{name:"a_labelAngle",size:1,type:_},{name:"a_backdropColor",size:4,type:_},{name:"a_backdropShadowColor",size:4,type:_},{name:"a_backdropShadowBlur",size:1,type:_},{name:"a_backdropPadding",size:1,type:_},{name:"a_backdropBorderColor",size:4,type:_},{name:"a_backdropExtra",size:4,type:_},{name:"a_labelBoxOffset",size:2,type:_}],CONSTANT_ATTRIBUTES:[{name:"a_quadCorner",size:2,type:_}],CONSTANT_DATA:[[-1,-1],[1,-1],[-1,1],[1,1]]}}},{key:"processBackdrop",value:function(x,_){var T=this.floats,R=this.STRIDE,A=x*R;T[A++]=_.nodeIndex,T[A++]=_.labelWidth,T[A++]=_.labelHeight,T[A++]=_.textHeight,T[A++]=ln[_.position],T[A++]=_.labelAngle,T[A++]=_.backdropColor[0],T[A++]=_.backdropColor[1],T[A++]=_.backdropColor[2],T[A++]=_.backdropColor[3],T[A++]=_.backdropShadowColor[0],T[A++]=_.backdropShadowColor[1],T[A++]=_.backdropShadowColor[2],T[A++]=_.backdropShadowColor[3],T[A++]=_.backdropShadowBlur,T[A++]=_.backdropPadding,T[A++]=_.backdropBorderColor[0],T[A++]=_.backdropBorderColor[1],T[A++]=_.backdropBorderColor[2],T[A++]=_.backdropBorderColor[3],T[A++]=_.backdropBorderWidth,T[A++]=_.backdropCornerRadius,T[A++]=_.backdropLabelPadding,T[A++]=_.backdropArea,T[A++]=_.labelBoxOffset[0],T[A++]=_.labelBoxOffset[1]}},{key:"setUniforms",value:function(x,_){var T=_.gl,R=_.uniformLocations;T.uniformMatrix3fv(R.u_matrix,!1,x.matrix),T.uniform1f(R.u_sizeRatio,x.sizeRatio),T.uniform1f(R.u_correctionRatio,x.correctionRatio),T.uniform1f(R.u_cameraAngle,x.cameraAngle),T.uniform2f(R.u_resolution,x.width*x.pixelRatio,x.height*x.pixelRatio),T.uniform1f(R.u_pixelRatio,x.pixelRatio),T.uniform1f(R.u_labelMargin,m.labelMargin),T.uniform1f(R.u_zoomLabelSizeRatio,1/h(x.zoomRatio)),T.uniform1f(R.u_labelPixelSnapping,x.labelPixelSnapping),T.uniform1i(R.u_nodeDataTexture,x.nodeDataTextureUnit),T.uniform1i(R.u_nodeDataTextureWidth,x.nodeDataTextureWidth),T.uniform1i(R.u_nodeFrameTexture,x.nodeFrameTextureUnit),T.uniform1i(R.u_nodeFrameTextureWidth,x.nodeFrameTextureWidth);var A=d();R.u_layerAttributeTexture&&A&&(A.bind(mt),T.uniform1i(R.u_layerAttributeTexture,mt),T.uniform1i(R.u_layerAttributeTextureWidth,A.getTextureWidth()),T.uniform1i(R.u_layerAttributeTexelsPerNode,A.getTexelsPerItem()));var S=U(Pn(l)),k;try{for(S.s();!(k=S.n()).done;){var L=k.value;this.setTypedUniform(L,_)}}catch(C){S.e(C)}finally{S.f()}}},{key:"hasNothingToRender",value:function(){return this.totalBackdropCount===0}},{key:"drawWebGL",value:function(x,_){var T=_.gl;this.totalBackdropCount!==0&&(this.isInstanced?T.drawArraysInstanced(x,0,this.VERTICES,this.totalBackdropCount):T.drawArrays(x,0,this.totalBackdropCount*this.VERTICES))}},{key:"reallocate",value:function(x){this.totalBackdropCount=x,x>this.bufferCapacity&&(this.bufferCapacity=Math.max(x,Math.ceil(this.bufferCapacity*1.5)||10),Ee(m,"reallocate",this,3)([this.bufferCapacity]))}}])})(Gt);return D(v,"labelMargin",g),new v(e,n,t)}var Ko=(function(e){function n(){var t;ee(this,n);for(var r=arguments.length,a=new Array(r),i=0;i<r;i++)a[i]=arguments[i];return t=ge(this,n,[].concat(a)),D(t,"totalCharacterCount",0),D(t,"bufferCapacity",0),t}return pe(n,e),te(n,[{key:"processLabel",value:function(r,a,i){if(i.hidden||!i.text)return 0;for(var o=i.text,s=o.length,l=0;l<s;l++){var u=o[l];this.processCharacter(a+l,i,u,l)}return s}},{key:"hasNothingToRender",value:function(){return this.totalCharacterCount===0}},{key:"drawWebGL",value:function(r,a){var i=a.gl;this.totalCharacterCount!==0&&(this.isInstanced?i.drawArraysInstanced(r,0,this.VERTICES,this.totalCharacterCount):i.drawArrays(r,0,this.totalCharacterCount*this.VERTICES))}},{key:"reallocate",value:function(r){this.totalCharacterCount=r,r>this.bufferCapacity&&(this.bufferCapacity=Math.max(r,Math.ceil(this.bufferCapacity*1.5)||1e3),Ee(n,"reallocate",this,3)([this.bufferCapacity]))}}])})(Gt);function Kd(){var e=`#version 300 es

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

`.concat(ct,`
`).concat(It,`
`).concat(cn,`
`).concat(qo,`

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

  `).concat($o,`

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
`);return e}var Zd=`#version 300 es
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
`;function Qd(e,n,t,r){var a,i,o=r.label,s=o===void 0?{}:o,l=(a=s.margin)!==null&&a!==void 0?a:un,u=(i=s.zoomToLabelSizeRatioFunction)!==null&&i!==void 0?i:function(){return 1},d=Kd(),c=(function(g){function h(){var f;ee(this,h);for(var y=arguments.length,v=new Array(y),p=0;p<y;p++)v[p]=arguments[p];return f=ge(this,h,[].concat(v)),D(f,"totalCount",0),D(f,"bufferCapacity",0),f}return pe(h,g),te(h,[{key:"getDefinition",value:function(){var y=WebGL2RenderingContext,v=y.FLOAT,p=y.UNSIGNED_BYTE,m=y.TRIANGLE_STRIP;return{VERTICES:4,VERTEX_SHADER_SOURCE:d,FRAGMENT_SHADER_SOURCE:Zd,METHOD:m,UNIFORMS:["u_matrix","u_sizeRatio","u_correctionRatio","u_cameraAngle","u_resolution","u_pixelRatio","u_labelMargin","u_zoomLabelSizeRatio","u_labelPixelSnapping","u_pickingPadding","u_nodeDataTexture","u_nodeDataTextureWidth","u_nodeFrameTexture","u_nodeFrameTextureWidth"],ATTRIBUTES:[{name:"a_nodeIndex",size:1,type:v},{name:"a_id",size:4,type:p,normalized:!0},{name:"a_color",size:4,type:p,normalized:!0},{name:"a_labelWidth",size:1,type:v},{name:"a_labelHeight",size:1,type:v},{name:"a_textHeight",size:1,type:v},{name:"a_positionMode",size:1,type:v},{name:"a_labelAngle",size:1,type:v},{name:"a_padding",size:1,type:v}],CONSTANT_ATTRIBUTES:[{name:"a_quadCorner",size:2,type:v}],CONSTANT_DATA:[[-1,-1],[1,-1],[-1,1],[1,1]]}}},{key:"processLabelBackground",value:function(y,v){var p=this.floats,m=this.ints,b=y*this.STRIDE;p[b++]=v.nodeIndex,m[b++]=v.id,p[b++]=v.color,p[b++]=v.labelWidth,p[b++]=v.labelHeight,p[b++]=v.textHeight,p[b++]=v.positionMode,p[b++]=v.labelAngle,p[b++]=v.padding}},{key:"setUniforms",value:function(y,v){var p=v.gl,m=v.uniformLocations;p.uniformMatrix3fv(m.u_matrix,!1,y.matrix),p.uniform1f(m.u_sizeRatio,y.sizeRatio),p.uniform1f(m.u_correctionRatio,y.correctionRatio),p.uniform1f(m.u_cameraAngle,y.cameraAngle),p.uniform2f(m.u_resolution,y.width*y.pixelRatio,y.height*y.pixelRatio),p.uniform1f(m.u_pixelRatio,y.pixelRatio),p.uniform1f(m.u_labelMargin,h.labelMargin),p.uniform1f(m.u_zoomLabelSizeRatio,1/u(y.zoomRatio)),p.uniform1f(m.u_labelPixelSnapping,y.labelPixelSnapping),p.uniform1f(m.u_pickingPadding,y.labelPickingPadding),p.uniform1i(m.u_nodeDataTexture,y.nodeDataTextureUnit),p.uniform1i(m.u_nodeDataTextureWidth,y.nodeDataTextureWidth),p.uniform1i(m.u_nodeFrameTexture,y.nodeFrameTextureUnit),p.uniform1i(m.u_nodeFrameTextureWidth,y.nodeFrameTextureWidth)}},{key:"hasNothingToRender",value:function(){return this.totalCount===0}},{key:"drawWebGL",value:function(y,v){var p=v.gl;this.totalCount!==0&&p.drawArraysInstanced(p.TRIANGLE_STRIP,0,this.VERTICES,this.totalCount)}},{key:"reallocate",value:function(y){this.totalCount=y,y>this.bufferCapacity&&(this.bufferCapacity=Math.max(y,Math.ceil(this.bufferCapacity*1.5)||10),Ee(h,"reallocate",this,3)([this.bufferCapacity]))}}])})(Gt);return D(c,"labelMargin",l),new c(e,n,t)}var Ze={fontSize:64,buffer:8,radius:24,cutoff:.25,maxTextureSize:2048,debounceTimeout:100},wn=2,Cn=1e20;function eh(e,n,t,r,a){var i=e+a*4,o=document.createElement("canvas");o.width=i,o.height=i;var s=o.getContext("2d",{willReadFrequently:!0});return s.font="".concat(r," ").concat(t," ").concat(e,"px ").concat(n),s.textBaseline="alphabetic",s.textAlign="left",s.fillStyle="black",{ctx:s,canvasSize:i,gridOuter:new Float64Array(i*i),gridInner:new Float64Array(i*i),f:new Float64Array(i),z:new Float64Array(i+1),v:new Uint16Array(i)}}function th(e,n,t,r,a){var i=e.ctx,o=e.canvasSize,s=i.measureText(n),l=s.width,u=s.actualBoundingBoxAscent,d=s.actualBoundingBoxDescent,c=s.actualBoundingBoxLeft,g=s.actualBoundingBoxRight,h=Math.ceil(u),f=Math.ceil(c),y=Math.max(0,Math.min(o-t,Math.ceil(c)+Math.ceil(g))),v=Math.min(o-t,h+Math.ceil(d)),p=y+2*t,m=v+2*t,b=Math.max(p*m,0),x=new Uint8ClampedArray(b),_={data:x,width:p,height:m,glyphWidth:y,glyphHeight:v,glyphTop:h,glyphLeft:f,glyphAdvance:l};if(y===0||v===0)return _;var T=e.gridInner,R=e.gridOuter;i.clearRect(t,t,y,v),i.fillText(n,t+f,t+h);var A=i.getImageData(t,t,y,v);R.fill(Cn,0,b),T.fill(0,0,b);for(var S=0;S<v;S++)for(var k=0;k<y;k++){var L=A.data[4*(S*y+k)+3]/255;if(L!==0){var C=(S+t)*p+k+t;if(L===1)R[C]=0,T[C]=Cn;else{var z=.5-L;R[C]=z>0?z*z:0,T[C]=z<0?z*z:0}}}Io(R,0,0,p,m,p,e.f,e.v,e.z),Io(T,t,t,y,v,p,e.f,e.v,e.z);for(var F=0;F<b;F++){var P=Math.sqrt(R[F])-Math.sqrt(T[F]);x[F]=Math.round(255-255*(P/r+a))}return _}function Io(e,n,t,r,a,i,o,s,l){for(var u=n;u<n+r;u++)Go(e,t*i+u,i,a,o,s,l);for(var d=t;d<t+a;d++)Go(e,d*i+n,1,r,o,s,l)}function Go(e,n,t,r,a,i,o){i[0]=0,o[0]=-Cn,o[1]=Cn,a[0]=e[n];for(var s=1,l=0,u=0;s<r;s++){a[s]=e[n+s*t];var d=s*s;do{var c=i[l];u=(a[s]-a[c]+d-c*c)/(s-c)/2}while(u<=o[l]&&--l>-1);l++,i[l]=s,o[l]=u,o[l+1]=Cn}for(var g=0,h=0;g<r;g++){for(;o[h+1]<g;)h++;var f=i[h],y=g-f;e[n+g*t]=a[f]+y*y}}var Pt=(function(e){function n(){var t,r=arguments.length>0&&arguments[0]!==void 0?arguments[0]:{};return ee(this,n),t=ge(this,n),D(t,"fonts",new Map),D(t,"textures",[]),D(t,"cursor",{x:0,y:0,rowHeight:0,atlasIndex:0}),D(t,"pendingGlyphs",[]),D(t,"debounceTimer",null),t.options=O(O({},Ze),r),t.canvas=document.createElement("canvas"),t.canvas.width=t.options.maxTextureSize,t.canvas.height=t.options.maxTextureSize,t.ctx=t.canvas.getContext("2d",{willReadFrequently:!0}),t.measureCanvas=document.createElement("canvas"),t.measureCtx=t.measureCanvas.getContext("2d"),t.textures.push(t.ctx.getImageData(0,0,1,1)),t}return pe(n,e),te(n,[{key:"getFontKey",value:function(r){return"".concat(r.family,"-").concat(r.weight,"-").concat(r.style)}},{key:"registerFont",value:function(r){var a=this.getFontKey(r);if(this.fonts.has(a))return a;var i=eh(this.options.fontSize,r.family,r.weight,r.style,this.options.buffer);return this.fonts.set(a,{descriptor:r,generator:i,glyphs:new Map}),a}},{key:"ensureGlyphs",value:function(r,a){var i=this.fonts.get(a);if(!i)throw new Error('Font "'.concat(a,'" is not registered. Call registerFont() first.'));var o=!1,s=U(r),l;try{for(s.s();!(l=s.n()).done;){var u=l.value,d=u.codePointAt(0);d!==void 0&&(i.glyphs.has(d)||(this.pendingGlyphs.push({fontKey:a,charCode:d}),o=!0))}}catch(c){s.e(c)}finally{s.f()}o&&this.scheduleTextureGeneration()}},{key:"measureText",value:function(r,a){var i=this.fonts.get(a);if(!i)throw new Error('Font "'.concat(a,'" is not registered.'));var o=i.descriptor,s=o.family,l=o.weight,u=o.style;return this.measureCtx.font="".concat(u," ").concat(l," ").concat(this.options.fontSize,"px ").concat(s),this.measureCtx.measureText(r).width}},{key:"getGlyph",value:function(r,a){var i=this.fonts.get(a);if(i)return i.glyphs.get(r)}},{key:"getTextures",value:function(){return this.textures}},{key:"getFontCount",value:function(){return this.fonts.size}},{key:"getGlyphCount",value:function(){var r=0,a=U(this.fonts.values()),i;try{for(a.s();!(i=a.n()).done;){var o=i.value;r+=o.glyphs.size}}catch(s){a.e(s)}finally{a.f()}return r}},{key:"hasPendingGlyphs",value:function(){return this.pendingGlyphs.length>0}},{key:"flush",value:function(){this.debounceTimer&&(clearTimeout(this.debounceTimer),this.debounceTimer=null),this.generateTextures()}},{key:"destroy",value:function(){this.debounceTimer&&clearTimeout(this.debounceTimer),this.fonts.clear(),this.textures=[],this.pendingGlyphs=[],this.removeAllListeners()}},{key:"scheduleTextureGeneration",value:function(){var r=this;this.debounceTimer===null&&(this.options.debounceTimeout===null?this.generateTextures():this.debounceTimer=setTimeout(function(){r.debounceTimer=null,r.generateTextures()},this.options.debounceTimeout))}},{key:"generateTextures",value:function(){if(this.pendingGlyphs.length!==0){var r=this.options,a=r.maxTextureSize,i=r.buffer,o=r.radius,s=r.cutoff,l=U(this.pendingGlyphs),u;try{for(l.s();!(u=l.n()).done;){var d=u.value,c=d.fontKey,g=d.charCode,h=this.fonts.get(c);if(!(!h||h.glyphs.has(g))){var f=String.fromCodePoint(g),y=th(h.generator,f,i,o,s),v=y.width,p=y.height;this.cursor.x+v+wn>a&&(this.cursor.x=0,this.cursor.y+=this.cursor.rowHeight+wn,this.cursor.rowHeight=0),this.cursor.y+p+wn>a&&(this.finalizeCurrentTexture(),this.cursor={x:0,y:0,rowHeight:0,atlasIndex:this.cursor.atlasIndex+1},this.ctx.clearRect(0,0,a,a));for(var m=y.data,b=new Uint8ClampedArray(v*p*4),x=0;x<m.length;x++){var _=x*4;b[_]=255,b[_+1]=255,b[_+2]=255,b[_+3]=m[x]}var T=new ImageData(b,v,p);this.ctx.putImageData(T,this.cursor.x,this.cursor.y);var R={charCode:g,width:y.glyphWidth,height:y.glyphHeight,bearingX:-y.glyphLeft-i,bearingY:y.glyphTop+i,advance:y.glyphAdvance,atlasX:this.cursor.x,atlasY:this.cursor.y,atlasWidth:v,atlasHeight:p,atlasIndex:this.cursor.atlasIndex};h.glyphs.set(g,R),this.cursor.x+=v+wn,this.cursor.rowHeight=Math.max(this.cursor.rowHeight,p)}}}catch(A){l.e(A)}finally{l.f()}this.finalizeCurrentTexture(),this.pendingGlyphs=[],this.emit(n.ATLAS_UPDATED_EVENT,{textures:this.textures,glyphCount:this.getGlyphCount()})}}},{key:"finalizeCurrentTexture",value:function(){var r=this.options.maxTextureSize,a=this.cursor.y>0||this.cursor.rowHeight>0,i=Math.min(r,Math.max(this.cursor.x,a?r:1)),o=Math.min(r,this.cursor.y+this.cursor.rowHeight+wn),s=this.ctx.getImageData(0,0,i,o);this.cursor.atlasIndex>=this.textures.length?this.textures.push(s):this.textures[this.cursor.atlasIndex]=s}}])})(Wo.EventEmitter);D(Pt,"ATLAS_UPDATED_EVENT","atlasUpdated");var nh=Ze.fontSize;function rh(){var e=`  // -------------------------------------------------------------------------
  // Step 3: Calculate position offset from the shared edge distance
  // -------------------------------------------------------------------------
  vec2 positionOffset = vec2(0.0);

  if (a_positionMode < 4.0) {
    vec2 screenDir = getLabelDirection(a_positionMode);
    float boundaryDistPixels = nodeRadiusPixels * edgeDist;
    positionOffset = screenDir * (boundaryDistPixels + margin);
  }`,n=`#version 300 es

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
const float ATLAS_FONT_SIZE = `.concat(ye(nh),`;

// ============================================================================
// Helper Functions
// ============================================================================

`).concat(ct,`
`).concat(It,`
`).concat(cn,`
`).concat(Vo,`

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
`);return n}function ah(){var e=`#version 300 es
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
`;return e}function ih(){return["u_matrix","u_sizeRatio","u_correctionRatio","u_cameraAngle","u_resolution","u_atlasSize","u_atlas","u_gamma","u_sdfBuffer","u_pixelRatio","u_nodeDataTexture","u_nodeDataTextureWidth","u_nodeFrameTexture","u_nodeFrameTextureWidth","u_zoomLabelSizeRatio","u_labelPixelSnapping"]}function oh(){return{vertexShader:rh(),fragmentShader:ah(),uniforms:ih()}}function sh(e,n,t,r){var a,i,o=r.label,s=o===void 0?{}:o,l=(a=s.margin)!==null&&a!==void 0?a:un,u=(i=s.zoomToLabelSizeRatioFunction)!==null&&i!==void 0?i:function(){return 1},d=oh(),c=(function(g){function h(f,y,v){var p,m,b,x;if(ee(this,h),x=ge(this,h,[f,y,v]),D(x,"atlasTexture",null),D(x,"atlasNeedsUpdate",!1),D(x,"labelGlyphCache",new Map),x.atlasFontSize=Ze.fontSize*Dn(),x.atlasManager=new Pt({fontSize:x.atlasFontSize}),x.gamma=.025,x.sdfBuffer=Ze.cutoff,x.atlasTexture=f.createTexture(),!x.atlasTexture)throw new Error("NodeLabelProgram: failed to create atlas texture");f.bindTexture(f.TEXTURE_2D,x.atlasTexture),f.texParameteri(f.TEXTURE_2D,f.TEXTURE_WRAP_S,f.CLAMP_TO_EDGE),f.texParameteri(f.TEXTURE_2D,f.TEXTURE_WRAP_T,f.CLAMP_TO_EDGE),f.texParameteri(f.TEXTURE_2D,f.TEXTURE_MIN_FILTER,f.LINEAR),f.texParameteri(f.TEXTURE_2D,f.TEXTURE_MAG_FILTER,f.LINEAR),f.bindTexture(f.TEXTURE_2D,null),x.atlasManager.on(Pt.ATLAS_UPDATED_EVENT,function(){x.atlasNeedsUpdate=!0});var _={family:((p=s.font)===null||p===void 0?void 0:p.family)||"sans-serif",weight:((m=s.font)===null||m===void 0?void 0:m.weight)||"normal",style:((b=s.font)===null||b===void 0?void 0:b.style)||"normal"};return x.defaultFontKey=x.atlasManager.registerFont(_),x}return pe(h,g),te(h,[{key:"getDefinition",value:function(){var y=WebGL2RenderingContext,v=y.FLOAT,p=y.UNSIGNED_BYTE,m=y.TRIANGLE_STRIP;return{VERTICES:4,VERTEX_SHADER_SOURCE:d.vertexShader,FRAGMENT_SHADER_SOURCE:d.fragmentShader,METHOD:m,UNIFORMS:d.uniforms,ATTRIBUTES:[{name:"a_nodeIndex",size:1,type:v},{name:"a_charOffset",size:2,type:v},{name:"a_charSize",size:2,type:v},{name:"a_texCoords",size:4,type:v},{name:"a_color",size:4,type:p,normalized:!0},{name:"a_margin",size:1,type:v},{name:"a_positionMode",size:1,type:v},{name:"a_labelWidth",size:1,type:v},{name:"a_labelHeight",size:1,type:v},{name:"a_verticalCenter",size:1,type:v},{name:"a_textHeight",size:1,type:v},{name:"a_labelAngle",size:1,type:v}],CONSTANT_ATTRIBUTES:[{name:"a_quadCorner",size:2,type:v}],CONSTANT_DATA:[[-1,-1],[1,-1],[-1,1],[1,1]]}}},{key:"prepareLabelGlyphs",value:function(y,v){if(v.hidden||!v.text){this.labelGlyphCache.delete(y);return}var p=v.text,m=v.fontKey||this.defaultFontKey;this.atlasManager.ensureGlyphs(p,m);var b=[],x=[],_=0,T=0,R=0,A=U(p),S;try{for(A.s();!(S=A.n()).done;){var k=S.value,L=k.codePointAt(0);if(L===void 0){b.push(void 0),x.push(_);continue}var C=this.atlasManager.getGlyph(L,m);b.push(C),x.push(_),C&&(_+=C.advance,T=Math.max(T,C.bearingY),R=Math.max(R,C.atlasHeight-C.bearingY))}}catch(z){A.e(z)}finally{A.f()}this.labelGlyphCache.set(y,{glyphs:b,xOffsets:x,totalWidth:_,totalHeight:T+R,verticalCenterOffset:(T-R)/2})}},{key:"processCharacter",value:function(y,v,p,m){var b=this.floats,x=this.STRIDE,_=y*x,T=this.labelGlyphCache.get(v.parentKey);if(!T||!T.glyphs[m]){for(var R=0;R<x;R++)b[_+R]=0;return}var A=T.glyphs[m],S=T.xOffsets[m],k=v.size/this.atlasFontSize,L=We(v.color),C=_;b[C++]=v.nodeIndex,b[C++]=(S+A.bearingX)*k,b[C++]=-A.bearingY*k,b[C++]=A.atlasWidth*k,b[C++]=A.atlasHeight*k,b[C++]=A.atlasX,b[C++]=A.atlasY,b[C++]=A.atlasWidth,b[C++]=A.atlasHeight,b[C++]=L,b[C++]=v.margin,b[C++]=ln[v.position],b[C++]=T.totalWidth*k,b[C++]=v.size,b[C++]=T.verticalCenterOffset*k,b[C++]=T.totalHeight*k,b[C++]=v.labelAngle}},{key:"processLabel",value:function(y,v,p){return this.prepareLabelGlyphs(y,p),Ee(h,"processLabel",this,3)([y,v,p])}},{key:"updateAtlasTexture",value:function(){if(this.atlasNeedsUpdate){var y=this.normalProgram.gl,v=this.atlasManager.getTextures();if(v.length!==0){var p=v[0];y.bindTexture(y.TEXTURE_2D,this.atlasTexture),y.texImage2D(y.TEXTURE_2D,0,y.RGBA,p.width,p.height,0,y.RGBA,y.UNSIGNED_BYTE,p.data),y.bindTexture(y.TEXTURE_2D,null),this.atlasNeedsUpdate=!1}}}},{key:"setUniforms",value:function(y,v){var p=v.gl,m=v.uniformLocations;p.uniformMatrix3fv(m.u_matrix,!1,y.matrix),p.uniform1f(m.u_sizeRatio,y.sizeRatio),p.uniform1f(m.u_correctionRatio,y.correctionRatio),p.uniform1f(m.u_cameraAngle,y.cameraAngle),p.uniform2f(m.u_resolution,y.width*y.pixelRatio,y.height*y.pixelRatio);var b=this.atlasManager.getTextures();b.length>0?p.uniform2f(m.u_atlasSize,b[0].width,b[0].height):p.uniform2f(m.u_atlasSize,1,1),p.activeTexture(p.TEXTURE0),p.bindTexture(p.TEXTURE_2D,this.atlasTexture),p.uniform1i(m.u_atlas,0),m.u_nodeDataTexture!==void 0&&p.uniform1i(m.u_nodeDataTexture,y.nodeDataTextureUnit),m.u_nodeDataTextureWidth!==void 0&&p.uniform1i(m.u_nodeDataTextureWidth,y.nodeDataTextureWidth),p.uniform1i(m.u_nodeFrameTexture,y.nodeFrameTextureUnit),p.uniform1i(m.u_nodeFrameTextureWidth,y.nodeFrameTextureWidth),p.uniform1f(m.u_gamma,this.gamma),p.uniform1f(m.u_sdfBuffer,this.sdfBuffer),p.uniform1f(m.u_pixelRatio,y.pixelRatio),p.uniform1f(m.u_zoomLabelSizeRatio,1/h.zoomToLabelSizeRatioFunction(y.zoomRatio)),p.uniform1f(m.u_labelPixelSnapping,y.labelPixelSnapping)}},{key:"renderProgram",value:function(y,v){this.updateAtlasTexture(),this.atlasManager.hasPendingGlyphs()&&(this.atlasManager.flush(),this.updateAtlasTexture()),Ee(h,"renderProgram",this,3)([y,v])}},{key:"registerFont",value:function(y){var v=arguments.length>1&&arguments[1]!==void 0?arguments[1]:"normal",p=arguments.length>2&&arguments[2]!==void 0?arguments[2]:"normal";return this.atlasManager.registerFont({family:y,weight:v,style:p})}},{key:"getAtlasManager",value:function(){return this.atlasManager}},{key:"measureLabel",value:function(y,v,p){var m=p||this.defaultFontKey;this.atlasManager.ensureGlyphs(y,m),this.atlasManager.hasPendingGlyphs()&&this.atlasManager.flush();var b=0,x=0,_=0,T=U(y),R;try{for(T.s();!(R=T.n()).done;){var A=R.value,S=A.codePointAt(0);if(S!==void 0){var k=this.atlasManager.getGlyph(S,m);k&&(b+=k.advance,x=Math.max(x,k.bearingY),_=Math.max(_,k.atlasHeight-k.bearingY))}}}catch(C){T.e(C)}finally{T.f()}var L=v/this.atlasFontSize;return{width:b*L,height:v,textHeight:(x+_)*L}}},{key:"ensureGlyphsReady",value:function(y,v){var p=v||this.defaultFontKey,m=U(y),b;try{for(m.s();!(b=m.n()).done;){var x=b.value;this.atlasManager.ensureGlyphs(x,p)}}catch(_){m.e(_)}finally{m.f()}this.atlasManager.flush()}},{key:"kill",value:function(){var y=this.normalProgram.gl;this.atlasTexture&&(y.deleteTexture(this.atlasTexture),this.atlasTexture=null),this.atlasManager.destroy(),this.labelGlyphCache.clear(),Ee(h,"kill",this,3)([])}}])})(Ko);return D(c,"labelMargin",l),D(c,"zoomToLabelSizeRatioFunction",u),new c(e,n,t)}var lh=`#version 300 es
precision highp float;

in float v_edgeDist;

// R32F target: only the .r channel is stored.
out vec4 fragColor;

void main() {
  fragColor = vec4(v_edgeDist, 0.0, 0.0, 0.0);
}
`;function uh(e,n,t){var r=za(e),a=Pn(e).map(function(f){return"uniform ".concat(f.type," ").concat(f.name,";")}).join(`
`),i=Pd(e,t),o=i.code,s=i.multiShape,l=dn(e,n),u=Object.keys(l.specs).map(function(f){return"float v_".concat(f,";")}).join(`
`),d=sn(l,{varPrefix:"nodeAttr",baseTexelExpr:"nodeIdx * u_layerAttributeTexelsPerNode",textureWidthUniform:"u_layerAttributeTextureWidth",textureSamplerUniform:"u_layerAttributeTexture"}),c=d.fetchCode,g=d.varyingAssignments,h=`#version 300 es
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

`).concat(ct,`
`).concat(It,`
`).concat(r,`
`).concat(o,`
`).concat(Vo,`

void main() {
  int nodeIdx = int(a_nodeIndex);
`).concat(c,`
`).concat(g,`
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
`);return h}var Zo=(function(){function e(n,t){ee(this,e),D(this,"uniformLocations",{});var r=t.shapes,a=t.layers,i=t.shapeGlobalIds;if(r.length===0)throw new Error("NodeLabelFramePass: at least one shape must be provided");this.gl=n,this.shapeUniforms=Pn(r),this.hasAttributeData=Object.keys(dn(r,a).offsets).length>0,this.vertexShader=Pa(n,uh(r,a,i)),this.fragmentShader=Ia(n,lh),this.program=Ga(n,[this.vertexShader,this.fragmentShader]);var o=["u_cameraAngle","u_nodeDataTexture","u_nodeDataTextureWidth","u_frameTextureWidth","u_frameTextureHeight","u_layerAttributeTexture","u_layerAttributeTextureWidth","u_layerAttributeTexelsPerNode"].concat(K(this.shapeUniforms.map(function(p){return p.name}))),s=U(o),l;try{for(s.s();!(l=s.n()).done;){var u=l.value;this.uniformLocations[u]=n.getUniformLocation(this.program,u)}}catch(p){s.e(p)}finally{s.f()}var d=e.FLOATS_PER_POINT*4;this.vao=n.createVertexArray(),this.buffer=n.createBuffer(),n.bindVertexArray(this.vao),n.bindBuffer(n.ARRAY_BUFFER,this.buffer);var c=U([["a_nodeIndex",0],["a_positionMode",4],["a_labelAngle",8]]),g;try{for(c.s();!(g=c.n()).done;){var h=re(g.value,2),f=h[0],y=h[1],v=n.getAttribLocation(this.program,f);v>=0&&(n.enableVertexAttribArray(v),n.vertexAttribPointer(v,1,n.FLOAT,!1,d,y))}}catch(p){c.e(p)}finally{c.f()}n.bindVertexArray(null)}return te(e,[{key:"run",value:function(t,r,a,i,o){if(r!==0){var s=this.gl;s.useProgram(this.program),s.bindVertexArray(this.vao),s.bindBuffer(s.ARRAY_BUFFER,this.buffer),s.bufferData(s.ARRAY_BUFFER,t.subarray(0,r*e.FLOATS_PER_POINT),s.DYNAMIC_DRAW);var l=this.uniformLocations;s.uniform1f(l.u_cameraAngle,i.cameraAngle),s.uniform1i(l.u_nodeDataTexture,i.nodeDataTextureUnit),s.uniform1i(l.u_nodeDataTextureWidth,i.nodeDataTextureWidth),s.uniform1f(l.u_frameTextureWidth,a.getTextureWidth()),s.uniform1f(l.u_frameTextureHeight,a.getTextureHeight()),this.hasAttributeData&&o&&(o.bind(mt),l.u_layerAttributeTexture&&s.uniform1i(l.u_layerAttributeTexture,mt),l.u_layerAttributeTextureWidth&&s.uniform1i(l.u_layerAttributeTextureWidth,o.getTextureWidth()),l.u_layerAttributeTexelsPerNode&&s.uniform1i(l.u_layerAttributeTexelsPerNode,o.getTexelsPerItem()));var u=U(this.shapeUniforms),d;try{for(u.s();!(d=u.n()).done;){var c=d.value;Ma(s,this.uniformLocations[c.name],c)}}catch(g){u.e(g)}finally{u.f()}a.bindAsRenderTarget(),s.disable(s.BLEND),s.disable(s.DEPTH_TEST),s.drawArrays(s.POINTS,0,r),s.bindFramebuffer(s.FRAMEBUFFER,null),s.bindVertexArray(null)}}},{key:"kill",value:function(){var t=this.gl;t.deleteProgram(this.program),t.deleteShader(this.vertexShader),t.deleteShader(this.fragmentShader),t.deleteBuffer(this.buffer),t.deleteVertexArray(this.vao)}}])})();D(Zo,"FLOATS_PER_POINT",3);function Qo(e,n,t,r,a){var i,o=r.label,s=o===void 0?{}:o,l=r.shapes;if(l.length===0)throw new Error("createNodeProgram: at least one shape must be provided in 'shapes'");var u={},d=[],c;l.forEach(function(R,A){var S=Ed(R);A===0&&(c=S),u[R.name]=A,d[A]=Ln(S)});var g=K(r.layers),h=Po({shapes:l,layers:g,shapeGlobalIds:l.length>1?d:void 0,antialias:a}),f=sh(e,null,t,{label:s}),y=Jd(e,null,t,{shapes:l,layers:g,getAttributeTexture:function(){return T.getAttributeTexture()},label:s,shapeGlobalIds:l.length>1?d:void 0}),v=Qd(e,n,t,{label:s}),p=r.labelAttachments&&Object.keys(r.labelAttachments).length>0?Ud(e,null,t,{label:s}):null,m=new Zo(e,{shapes:l,layers:g,shapeGlobalIds:l.length>1?d:void 0}),b=Hr(l,g),x=Oe(b),_=(i=(function(R){function A(S,k,L){var C;ee(this,A),C=ge(this,A,[S,k,L]),D(C,"layerLifecycles",new Map),D(C,"layersNeedingRegeneration",new Set),D(C,"attrDescriptors",[]),C._pickingBuffer=k;var z=_.layerTextures.get(S);z||(z=new Wr(S,x),_.layerTextures.set(S,z)),C.layerAttributeTexture=z;var F=_.textureRefCounts.get(S)||0;_.textureRefCounts.set(S,F+1),C.packedAttributeData=new Float32Array(x.floatsPerItem),g.forEach(function(N,G){if(N.lifecycle){var E={gl:S,renderer:{refresh:function(){return L.refresh()}},getUniformLocation:function(B){return S.getUniformLocation(C.normalProgram.program,B)},requestShaderRegeneration:function(){C.layersNeedingRegeneration.add(G)},requestRefresh:function(){L.refresh()}},I=N.lifecycle(E);C.layerLifecycles.set(G,I)}}),C.layerLifecycles.forEach(function(N){var G;(G=N.init)===null||G===void 0||G.call(N)});var P=new Map;return C.layerLifecycles.forEach(function(N,G){N.getAttributeData&&P.set(G,N)}),C.attrDescriptors=Ur(b,x,P),C}return pe(A,R),te(A,[{key:"getDefinition",value:function(){var k=WebGL2RenderingContext,L=k.FLOAT,C=k.TRIANGLE_STRIP;return{VERTICES:4,VERTEX_SHADER_SOURCE:h.vertexShader,FRAGMENT_SHADER_SOURCE:h.fragmentShader,METHOD:C,UNIFORMS:h.uniforms,ATTRIBUTES:h.attributes,CONSTANT_ATTRIBUTES:[{name:"a_quadCorner",size:2,type:L}],CONSTANT_DATA:[[-1,-1],[1,-1],[-1,1],[1,1]]}}},{key:"maybeRegenerateShaders",value:function(){var k=this;if(this.layersNeedingRegeneration.size!==0){g=g.map(function(G,E){if(k.layersNeedingRegeneration.has(E)){var I=k.layerLifecycles.get(E);if(I!=null&&I.regenerate){var W=I.regenerate();return O(O({},W),{},{lifecycle:G.lifecycle})}}return G}),this.layersNeedingRegeneration.clear(),h=Po({shapes:l,layers:g,shapeGlobalIds:l.length>1?d:void 0,antialias:this.renderer.getSetting("antialiasNodes")});var L=this.normalProgram.gl,C=this.normalProgram,z=C.program,F=C.buffer,P=C.vertexShader,N=C.fragmentShader;L.deleteProgram(z),L.deleteBuffer(F),L.deleteShader(P),L.deleteShader(N),this.normalProgram=this.getProgramInfo("normal",L,h.vertexShader,h.fragmentShader,this._pickingBuffer)}}},{key:"allocateNode",value:function(k){this.layerAttributeTexture.allocate(k)}},{key:"freeNode",value:function(k){this.layerAttributeTexture.free(k)}},{key:"getAttributeTexture",value:function(){return this.layerAttributeTexture}},{key:"uploadLayerTexture",value:function(){this.layerAttributeTexture.upload()}},{key:"process",value:function(k,L,C,z,F){var P=L*this.STRIDE;if(C.visibility==="hidden"){for(var N=P+this.STRIDE;P<N;P++)this.floats[P]=0;this.floats[L*this.STRIDE]=Xo;return}this.processVisibleItem(en(k),P,C,z,F)}},{key:"processVisibleItem",value:function(k,L,C,z,F){var P,N=this.floats,G=this.ints;if(N[L++]=z,G[L++]=k,N[L++]=(P=C.opacity)!==null&&P!==void 0?P:1,x.floatsPerItem!==0){var E=this.packedAttributeData;Or(this.attrDescriptors,C,E,C.color,this.layerLifecycles,0),this.layerAttributeTexture.updateAllAttributes(F,E)}}},{key:"setUniforms",value:function(k,L){var C=this,z=L.gl,F=L.uniformLocations;F.u_matrix&&z.uniformMatrix3fv(F.u_matrix,!1,k.matrix),F.u_sizeRatio&&z.uniform1f(F.u_sizeRatio,k.sizeRatio),F.u_correctionRatio&&z.uniform1f(F.u_correctionRatio,k.correctionRatio),F.u_pickingPadding&&z.uniform1f(F.u_pickingPadding,k.nodePickingPadding),F.u_cameraAngle&&z.uniform1f(F.u_cameraAngle,k.cameraAngle),F.u_nodeDataTexture&&z.uniform1i(F.u_nodeDataTexture,k.nodeDataTextureUnit),F.u_nodeDataTextureWidth&&z.uniform1i(F.u_nodeDataTextureWidth,k.nodeDataTextureWidth),F.u_layerAttributeTexture&&(this.layerAttributeTexture.bind(mt),z.uniform1i(F.u_layerAttributeTexture,mt)),F.u_layerAttributeTextureWidth&&z.uniform1i(F.u_layerAttributeTextureWidth,this.layerAttributeTexture.getTextureWidth()),F.u_layerAttributeTexelsPerNode&&z.uniform1i(F.u_layerAttributeTexelsPerNode,this.layerAttributeTexture.getTexelsPerItem()),l.forEach(function(P){P.uniforms.forEach(function(N){C.setTypedUniform(N,L)})}),g.forEach(function(P){P.uniforms.forEach(function(N){C.setTypedUniform(N,L)})})}},{key:"render",value:function(k,L,C){this.maybeRegenerateShaders(),Ee(A,"render",this,3)([k,L,C])}},{key:"renderProgram",value:function(k,L){var C=L.gl,z=L.program;C.useProgram(z),L===this.normalProgram&&this.layerLifecycles.forEach(function(F){var P;(P=F.beforeRender)===null||P===void 0||P.call(F)}),Ee(A,"renderProgram",this,3)([k,L])}},{key:"kill",value:function(){this.layerLifecycles.forEach(function(C){var z;(z=C.kill)===null||z===void 0||z.call(C)}),this.layerLifecycles.clear();var k=this.normalProgram.gl,L=(_.textureRefCounts.get(k)||1)-1;L<=0?(this.layerAttributeTexture.kill(),_.layerTextures.delete(k),_.textureRefCounts.delete(k)):_.textureRefCounts.set(k,L),Ee(A,"kill",this,3)([])}}])})(Gt),D(i,"layerTextures",new WeakMap),D(i,"textureRefCounts",new WeakMap),i),T=new _(e,null,t);return{nodeProgram:T,labelProgram:f,backdropProgram:y,labelBackgroundProgram:v,attachmentProgram:p,framePass:m,shapeSlug:c,shapeNameToIndex:l.length>1?u:void 0,shapeGlobalIds:l.length>1?d:void 0}}var bt=6;function Gn(e){var n=e.offsets,t=e.specs,r=e.floatsPerItem,a=Object.keys(n);if(a.length===0||r===0)return{uniformDeclarations:"",uniformNames:[],vertexVaryingDeclarations:"",fragmentVaryingDeclarations:"",fetchCode:"",varyingAssignments:""};var i=`
uniform sampler2D u_edgeAttributeTexture;
uniform int u_edgeAttributeTextureWidth;
uniform int u_edgeAttributeTexelsPerEdge;`,o=["u_edgeAttributeTexture","u_edgeAttributeTextureWidth","u_edgeAttributeTexelsPerEdge"],s=a.map(function(h){var f=t[h].size===1?"float":"vec".concat(t[h].size);return"".concat(f," v_").concat(h,";")}),l=s.map(function(h){return"out ".concat(h)}).join(`
`),u=s.map(function(h){return"in ".concat(h)}).join(`
`),d=sn(e,{varPrefix:"attr",baseTexelExpr:"edgeIdx * u_edgeAttributeTexelsPerEdge",textureWidthUniform:"u_edgeAttributeTextureWidth",textureSamplerUniform:"u_edgeAttributeTexture"}),c=d.fetchCode,g=d.varyingAssignments;return{uniformDeclarations:i,uniformNames:o,vertexVaryingDeclarations:l,fragmentVaryingDeclarations:u,fetchCode:c,varyingAssignments:g}}function ch(e){return`
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
`)}function dh(e){return`
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
`)}function es(e){return`
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
`)}function hh(e){return`
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
`)}function fh(e){return`
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
`)}function gh(e){return`
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
`)}function ph(e){return`
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
`)}function Ir(e,n){var t=new RegExp("\\b(float|vec[234]|void|int|bool)\\s+".concat(n,"\\s*\\("));return t.test(e)}function ts(e,n){var t=[];return Ir(n,"path_".concat(e,"_length"))||t.push(hh(e)),Ir(n,"path_".concat(e,"_closest_t"))||t.push(fh(e)),Ir(n,"path_".concat(e,"_distance"))||t.push(gh(e)),Ir(n,"path_".concat(e,"_t_at_distance"))||t.push(ph(e)),t.length>0?`
// ============================================================================
// Auto-generated fallback functions (path only provided position)
// ============================================================================
`.concat(t.join(`
`)):""}function ns(e){var n,t,r,a=e.paths,i=e.layers,o=(n=e.extremities)!==null&&n!==void 0?n:[];if(a.length===0)throw new Error("At least one path is required in 'paths'");if(i.length===0)throw new Error("At least one layer is required in 'layers'");var s=(t=e.defaultHead)!==null&&t!==void 0?t:"none",l=(r=e.defaultTail)!==null&&r!==void 0?r:"none";return{paths:a,extremities:o,layers:i,path:a[0],layer:i[0],defaultHead:s,defaultTail:l}}var rs=WebGL2RenderingContext,Fn=rs.FLOAT,zo=rs.UNSIGNED_BYTE;function vh(e,n,t){var r=new Set(["u_matrix","u_sizeRatio","u_correctionRatio","u_zoomRatio","u_pixelRatio","u_cameraAngle","u_minEdgeThickness","u_pickingPadding","u_nodeDataTexture","u_nodeDataTextureWidth","u_edgeDataTexture","u_edgeDataTextureWidth","u_edgeFrameTexture","u_edgeFrameTextureWidth","u_edgeAttributeTexture","u_edgeAttributeTextureWidth","u_edgeAttributeTexelsPerEdge"]),a=new Set(r);return e.forEach(function(i){return i.uniforms.forEach(function(o){return a.add(o.name)})}),n.forEach(function(i){return i.uniforms.forEach(function(o){return a.add(o.name)})}),t.forEach(function(i){return i.uniforms.forEach(function(o){return a.add(o.name)})}),Array.from(a)}function Wa(e){var n=new Set,t=[],r=U(e),a;try{for(r.s();!(a=r.n()).done;){var i=a.value;n.has(i.name)||(n.add(i.name),t.push("// Path: ".concat(i.name)),t.push(i.glsl),t.push(es(i.name)),t.push(ts(i.name,i.glsl)))}}catch(o){r.e(o)}finally{r.f()}return t.join(`

`)}function mh(e){var n=[],t=U(e),r;try{for(t.s();!(r=t.n()).done;){var a=r.value;n.push("// Extremity: ".concat(a.name)),n.push(a.glsl)}}catch(i){t.e(i)}finally{t.f()}return n.join(`

`)}var bh=[{queryName:"queryPathPosition",pathFunc:"position",returnType:"vec2",params:"float t, vec2 source, vec2 target",args:"t, source, target"},{queryName:"queryPathTangent",pathFunc:"tangent",returnType:"vec2",params:"float t, vec2 source, vec2 target",args:"t, source, target"},{queryName:"queryPathNormal",pathFunc:"normal",returnType:"vec2",params:"float t, vec2 source, vec2 target",args:"t, source, target"},{queryName:"queryPathLength",pathFunc:"length",returnType:"float",params:"vec2 source, vec2 target",args:"source, target"},{queryName:"queryPathClosestT",pathFunc:"closest_t",returnType:"float",params:"vec2 p, vec2 source, vec2 target",args:"p, source, target"}];function yh(e,n){var t=n.queryName,r=n.pathFunc,a=n.returnType,i=n.params,o=n.args;if(e.length===1)return"".concat(a," ").concat(t,"(int pathId, ").concat(i,`) {
  return path_`).concat(e[0].name,"_").concat(r,"(").concat(o,`);
}`);var s=e.map(function(l,u){return"    case ".concat(u,": return path_").concat(l.name,"_").concat(r,"(").concat(o,");")}).join(`
`);return"".concat(a," ").concat(t,"(int pathId, ").concat(i,`) {
  switch (pathId) {
`).concat(s,`
    default: return path_`).concat(e[0].name,"_").concat(r,"(").concat(o,`);
  }
}`)}function Ua(e){return bh.map(function(n){return yh(e,n)}).join(`

`)}function _h(e){if(e.length===1)return`float queryExtremitySDF(int extremityId, vec2 uv, float lengthRatio, float widthRatio) {
  return extremity_`.concat(e[0].name,`(uv, lengthRatio, widthRatio);
}`);var n=e.map(function(t,r){return"    case ".concat(r,": return extremity_").concat(t.name,"(uv, lengthRatio, widthRatio);")}).join(`
`);return`float queryExtremitySDF(int extremityId, vec2 uv, float lengthRatio, float widthRatio) {
  switch (extremityId) {
`.concat(n,`
    default: return extremity_`).concat(e[0].name,`(uv, lengthRatio, widthRatio);
  }
}`)}function xh(e){var n=[],t=U(e),r;try{for(t.s();!(r=t.n()).done;){var a=r.value;n.push(ch(a.name)),n.push(dh(a.name))}}catch(s){t.e(s)}finally{t.f()}if(e.length===1)n.push(`
float queryFindSourceClampT(int pathId, vec2 source, float sourceSize, int sourceShapeId, float sourceRotateAlign, vec2 target, float margin) {
  return findSourceClampT_`.concat(e[0].name,`(source, sourceSize, sourceShapeId, sourceRotateAlign, target, margin);
}

float queryFindTargetClampT(int pathId, vec2 source, vec2 target, float targetSize, int targetShapeId, float targetRotateAlign, float margin) {
  return findTargetClampT_`).concat(e[0].name,`(source, target, targetSize, targetShapeId, targetRotateAlign, margin);
}`));else{var i=e.map(function(s,l){return"    case ".concat(l,": return findSourceClampT_").concat(s.name,"(source, sourceSize, sourceShapeId, sourceRotateAlign, target, margin);")}).join(`
`),o=e.map(function(s,l){return"    case ".concat(l,": return findTargetClampT_").concat(s.name,"(source, target, targetSize, targetShapeId, targetRotateAlign, margin);")}).join(`
`);n.push(`
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
}`))}return n.join(`

`)}function Th(e,n,t,r,a){var i=Oe([].concat(K(e),K(t))),o=Gn(i),s=dn(r,a),l=Object.keys(s.offsets),u=function(R){var A=s.specs[R].size;return A===1?"float":"vec".concat(A)},d=l.map(function(T){return"".concat(u(T)," v_source_").concat(T,`;
`).concat(u(T)," v_target_").concat(T,";")}).join(`
`),c=sn(s,{varPrefix:"srcNodeAttr",baseTexelExpr:"srcIdx * u_layerAttributeTexelsPerNode",textureWidthUniform:"u_layerAttributeTextureWidth",textureSamplerUniform:"u_layerAttributeTexture",outputPrefix:"v_source_"}),g=sn(s,{varPrefix:"tgtNodeAttr",baseTexelExpr:"tgtIdx * u_layerAttributeTexelsPerNode",textureWidthUniform:"u_layerAttributeTextureWidth",textureSamplerUniform:"u_layerAttributeTexture",outputPrefix:"v_target_"}),h=l.map(function(T){return"".concat(u(T)," g_").concat(T,";")}).join(`
`),f=l.map(function(T){return"  g_".concat(T," = v_source_").concat(T,";")}).join(`
`),y=l.map(function(T){return"  g_".concat(T," = v_target_").concat(T,";")}).join(`
`),v=new Set,p=[],m=function(R){v.has(R.name)||(v.add(R.name),p.push("uniform ".concat(R.type," ").concat(R.name,";")))};e.forEach(function(T){return T.uniforms.forEach(m)}),n.forEach(function(T){return T.uniforms.forEach(m)});var b=n.map(function(T){return ye(T.widthFactor)}).join(", "),x=Math.max.apply(Math,K(e.map(function(T){return T.minBodyLengthRatio||0}))),_=`#version 300 es

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
`).concat(d,`
`).concat(h,`

// Scattered output written to the edge-frame texel: [tStart, tEnd, straightenFactor, pathLength]
out vec4 v_clamp;

// Extremity width factor array (needed for extremityScale computation)
const float EXTREMITY_WIDTH_FACTORS[`).concat(n.length,"] = float[](").concat(b,`);

// Node-data fetch helpers (geometry texel + rotation-flags texel)
`).concat(ct,`
`).concat(It,`

// Shape SDFs for node boundary clamping
`).concat(Sd(),`
`).concat(Rd(new Set(l)),`

// Path functions
`).concat(Wa(e),`
`).concat(Ua(e),`
`).concat(xh(e),`

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
`).concat(c.fetchCode,`
`).concat(c.varyingAssignments,`
`).concat(g.fetchCode,`
`).concat(g.varyingAssignments,`

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
  float minBodyLengthRatio = `).concat(ye(x),`;

  // Thickness in WebGL units (needed to compute clamping margin and extremity lengths)
  float pixelsThickness = max(a_thickness, u_minEdgeThickness * u_sizeRatio);
  float webGLThickness = pixelsThickness * u_correctionRatio / u_sizeRatio;

  // SDF clamping: find where the edge body meets the node boundaries. Always
  // searched (ungated) so labels get a true boundary clamp; the body re-applies
  // its extremity gating in-shader.
`).concat(f,`
  float tStart = queryFindSourceClampT(pathId, a_source, a_sourceSize, int(a_sourceShapeId), a_sourceRotateAlign, a_target, 0.0);
`).concat(y,`
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
`).concat(f,`
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
`).concat(y,`
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
`);return _}var Gr=0,Bo=1,zr=2;function Eh(e,n,t){var r=[];t&&(r.push([Gr,0,-1],[Gr,0,1]),r.push([Gr,1,-1],[Gr,1,1]));for(var a=0;a<=e;a++){var i=a/e;r.push([Bo,i,-1],[Bo,i,1])}return n&&(r.push([zr,0,-1],[zr,0,1]),r.push([zr,1,-1],[zr,1,1])),{data:r,attributes:[{name:"a_zone",size:1,type:Fn},{name:"a_zoneT",size:1,type:Fn},{name:"a_side",size:1,type:Fn}],verticesPerEdge:r.length}}function Sh(e,n,t,r){var a=Oe([].concat(K(e),K(t))),i=Gn(a),o=new Set(["u_matrix","u_sizeRatio","u_correctionRatio","u_zoomRatio","u_pixelRatio","u_cameraAngle","u_minEdgeThickness","u_pickingPadding","u_nodeDataTexture"]),s=new Set,l=[],u=function(p){!o.has(p.name)&&!s.has(p.name)&&(s.add(p.name),l.push("uniform ".concat(p.type," ").concat(p.name,";")))};e.forEach(function(v){return v.uniforms.forEach(u)}),n.forEach(function(v){return v.uniforms.forEach(u)}),t.forEach(function(v){return v.uniforms.forEach(u)});var d=r.map(function(v){var p=v.size===1?"float":"vec".concat(v.size);return"in ".concat(p," ").concat(v.name,";")}).join(`
`),c=n.map(function(v){return ye(v.widthFactor)}).join(", "),g=Math.max.apply(Math,K(e.map(function(v){return v.minBodyLengthRatio||0}))),h=t.some(function(v){return v.needsNodeColors}),f=e.some(function(v){return v.needsNodeSize}),y=`#version 300 es

// Constant attributes (per vertex)
`.concat(d,`

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
`).concat(f?`
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
const float EXTREMITY_WIDTH_FACTORS[`).concat(n.length,"] = float[](").concat(c,`);

// All path functions
`).concat(Wa(e),`

// Path selector functions
`).concat(Ua(e),`

// Node-data fetch helper (geometry texel of the two-texel node stride)
`).concat(ct,`
`).concat(h?Ld:"",`
// Per-edge clamp from the frame-pass (tStart, tEnd, straightenFactor, pathLength)
`).concat(cn,`

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
`).concat(f?`
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
  float minBodyLengthRatio = `).concat(ye(g),`;

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
`);return y}function Rh(e,n,t,r){var a=Oe([].concat(K(e),K(t))),i=Gn(a),o=new Set(["u_matrix","u_sizeRatio","u_correctionRatio","u_zoomRatio","u_pixelRatio","u_cameraAngle","u_minEdgeThickness","u_pickingPadding"]),s=new Set,l=[],u=function(m){!o.has(m.name)&&!s.has(m.name)&&(s.add(m.name),l.push("uniform ".concat(m.type," ").concat(m.name,";")))};e.forEach(function(p){return p.uniforms.forEach(u)}),n.forEach(function(p){return p.uniforms.forEach(u)}),t.forEach(function(p){return p.uniforms.forEach(u)});var d=n.map(function(p){var m;return ye((m=p.baseRatio)!==null&&m!==void 0?m:.5)}).join(", "),c=function(m){var b,x;return n.length>1?"EXTREMITY_BASE_RATIOS[".concat(m,"]"):ye((b=(x=n[0])===null||x===void 0?void 0:x.baseRatio)!==null&&b!==void 0?b:.5)},g=n.some(function(p){return lt(p.length)?!0:p.length>0}),h=t.some(function(p){return p.needsNodeColors}),f=e.some(function(p){return p.needsNodeSize}),y=t.map(function(p,m){return"  // Layer ".concat(m+1,": ").concat(p.name,`
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
`.concat(f?`
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

`).concat(n.length>1?`// Base ratio array for extremities (shared pool for head/tail)
const float EXTREMITY_BASE_RATIOS[`.concat(n.length,"] = float[](").concat(d,");"):"",`

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
`).concat(Wa(e),`

// Path selector functions
`).concat(Ua(e),`

// All extremity functions
`).concat(mh(n),`

// Extremity SDF selector (shared pool for head/tail)
`).concat(_h(n),`

// Layer functions
`).concat(t.map(function(p){return p.glsl}).join(`

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

`).concat(g?`  // Base ratios, from baseRatioLookup
  float headBaseRatio = `.concat(c("v_headId"),`;
  float tailBaseRatio = `).concat(c("v_tailId"),`;

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

`).concat(y,`

    // Mix with transparent to fade both color AND alpha (pre-multiplied alpha for correct blending)
    fragColor = mix(vec4(0.0), color, alpha);
  #endif
}
`);return v}function Ah(e,n,t){var r=lt(n.length)?!0:n.length>0,a=lt(t.length)?!0:t.length>0;if(e.generateConstantData){var i=e.generateConstantData();return{data:i.data,attributes:i.attributes,verticesPerEdge:i.verticesPerEdge}}return Eh(e.segments,r,a)}function No(e){var n,t=ns(e),r=t.paths,a=t.extremities,i=t.layers,o=(n=e.antialias)!==null&&n!==void 0?n:!0,s=new Map,l=new Map,u=new Map,d=U(r),c;try{for(d.s();!(c=d.n()).done;){var g=c.value,h=U(a),f;try{for(h.s();!(f=h.n()).done;){var y=f.value,v=U(a),p;try{for(v.s();!(p=v.n()).done;){var m=p.value,b="".concat(g.name,":").concat(y.name,":").concat(m.name),x=Ah(g,y,m);s.set(b,x.verticesPerEdge),l.set(b,x.data);var _=U(x.attributes),T;try{for(_.s();!(T=_.n()).done;){var R=T.value;u.has(R.name)||u.set(R.name,R)}}catch(j){_.e(j)}finally{_.f()}}}catch(j){v.e(j)}finally{v.f()}}}catch(j){h.e(j)}finally{h.f()}}}catch(j){d.e(j)}finally{d.f()}var A=Array.from(u.values()),S={};A.forEach(function(j,J){S[j.name]=J});var k=0,L="",C=U(s),z;try{for(C.s();!(z=C.n()).done;){var F=re(z.value,2),P=F[0],N=F[1];N>k&&(k=N,L=P)}}catch(j){C.e(j)}finally{C.f()}var G=l.get(L)||[],E=L.split(":"),I=re(E,1),W=I[0],B=r.find(function(j){return j.name===W}),w=[];B!=null&&B.generateConstantData?w=B.generateConstantData().attributes:w=[{name:"a_zone"},{name:"a_zoneT"},{name:"a_side"}];var $=G.map(function(j){var J=new Array(A.length).fill(0);return w.forEach(function(Se,le){var Y=S[Se.name];Y!==void 0&&le<j.length&&(J[Y]=j[le])}),J});return{vertexShader:Sh(r,a,i,A),fragmentShader:Rh(r,a,i,o),uniforms:vh(r,a,i),attributes:Dh(),verticesPerEdge:k,constantData:$,constantAttributes:A,vertexCountsPerCombination:s,constantDataPerCombination:l}}function Dh(e,n,t){return[{name:"a_edgeIndex",size:1,type:Fn},{name:"a_color",size:4,type:zo,normalized:!0},{name:"a_id",size:4,type:zo,normalized:!0},{name:"a_opacity",size:1,type:Fn}]}var wh=`#version 300 es
precision highp float;

in vec4 v_clamp;

// RGBA32F target: stores (tStart, tEnd, straightenFactor, pathLength).
out vec4 fragColor;

void main() {
  fragColor = v_clamp;
}
`,kh=(function(){function e(n,t){ee(this,e),D(this,"uniformLocations",{});var r=t.paths,a=t.extremities,i=t.layers,o=t.nodeShapes,s=t.nodeLayers;this.gl=n,this.hasAttributeData=Oe([].concat(K(r),K(i))).floatsPerItem>0,this.hasNodeAttributeData=Object.keys(dn(o,s).offsets).length>0,this.vertexShader=Pa(n,Th(r,a,i,o,s)),this.fragmentShader=Ia(n,wh),this.program=Ga(n,[this.vertexShader,this.fragmentShader]);var l=new Set;this.customUniforms=[];for(var u=0,d=[].concat(K(r),K(a));u<d.length;u++){var c=d[u],g=U(c.uniforms),h;try{for(g.s();!(h=g.n()).done;){var f=h.value;l.has(f.name)||(l.add(f.name),this.customUniforms.push(f))}}catch(b){g.e(b)}finally{g.f()}}var y=["u_sizeRatio","u_correctionRatio","u_cameraAngle","u_minEdgeThickness","u_nodeDataTexture","u_nodeDataTextureWidth","u_edgeDataTexture","u_edgeDataTextureWidth","u_edgeAttributeTexture","u_edgeAttributeTextureWidth","u_edgeAttributeTexelsPerEdge","u_layerAttributeTexture","u_layerAttributeTextureWidth","u_layerAttributeTexelsPerNode","u_frameTextureWidth","u_frameTextureHeight"].concat(K(this.customUniforms.map(function(b){return b.name}))),v=U(y),p;try{for(v.s();!(p=v.n()).done;){var m=p.value;this.uniformLocations[m]=n.getUniformLocation(this.program,m)}}catch(b){v.e(b)}finally{v.f()}this.vao=n.createVertexArray()}return te(e,[{key:"run",value:function(t,r,a,i,o){if(a!==0){var s=this.gl,l=this.uniformLocations;s.useProgram(this.program),s.bindVertexArray(this.vao),l.u_sizeRatio&&s.uniform1f(l.u_sizeRatio,t.sizeRatio),l.u_correctionRatio&&s.uniform1f(l.u_correctionRatio,t.correctionRatio),l.u_cameraAngle&&s.uniform1f(l.u_cameraAngle,t.cameraAngle),l.u_minEdgeThickness&&s.uniform1f(l.u_minEdgeThickness,t.minEdgeThickness),l.u_nodeDataTexture&&s.uniform1i(l.u_nodeDataTexture,t.nodeDataTextureUnit),l.u_nodeDataTextureWidth&&s.uniform1i(l.u_nodeDataTextureWidth,t.nodeDataTextureWidth),l.u_edgeDataTexture&&s.uniform1i(l.u_edgeDataTexture,t.edgeDataTextureUnit),l.u_edgeDataTextureWidth&&s.uniform1i(l.u_edgeDataTextureWidth,t.edgeDataTextureWidth),l.u_frameTextureWidth&&s.uniform1f(l.u_frameTextureWidth,r.getTextureWidth()),l.u_frameTextureHeight&&s.uniform1f(l.u_frameTextureHeight,r.getTextureHeight()),this.hasAttributeData&&i&&(i.bind(bt),l.u_edgeAttributeTexture&&s.uniform1i(l.u_edgeAttributeTexture,bt),l.u_edgeAttributeTextureWidth&&s.uniform1i(l.u_edgeAttributeTextureWidth,i.getTextureWidth()),l.u_edgeAttributeTexelsPerEdge&&s.uniform1i(l.u_edgeAttributeTexelsPerEdge,i.getTexelsPerItem())),this.hasNodeAttributeData&&o&&(o.bind(mt),l.u_layerAttributeTexture&&s.uniform1i(l.u_layerAttributeTexture,mt),l.u_layerAttributeTextureWidth&&s.uniform1i(l.u_layerAttributeTextureWidth,o.getTextureWidth()),l.u_layerAttributeTexelsPerNode&&s.uniform1i(l.u_layerAttributeTexelsPerNode,o.getTexelsPerItem()));var u=U(this.customUniforms),d;try{for(u.s();!(d=u.n()).done;){var c=d.value;Ma(s,this.uniformLocations[c.name],c)}}catch(g){u.e(g)}finally{u.f()}r.bindAsRenderTarget(),s.disable(s.BLEND),s.disable(s.DEPTH_TEST),s.drawArrays(s.POINTS,0,a),s.bindFramebuffer(s.FRAMEBUFFER,null),s.bindVertexArray(null)}}},{key:"kill",value:function(){var t=this.gl;t.deleteProgram(this.program),t.deleteShader(this.vertexShader),t.deleteShader(this.fragmentShader),t.deleteVertexArray(this.vao)}}])})();function kn(e,n,t,r,a,i){if(e.length===1)return"".concat(r," ").concat(n,"(int pathId, ").concat(a,`) {
  return path_`).concat(e[0].name,"_").concat(t,"(").concat(i,`);
}`);var o=e.map(function(s,l){return"    case ".concat(l,": return path_").concat(s.name,"_").concat(t,"(").concat(i,");")}).join(`
`);return"".concat(r," ").concat(n,"(int pathId, ").concat(a,`) {
  switch (pathId) {
`).concat(o,`
    default: return path_`).concat(e[0].name,"_").concat(t,"(").concat(i,`);
  }
}`)}var Ch=`
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
`;function Fh(e,n){var t=ye(e),r=ye(n);return`
float computeEdgeLabelAlpha(float bodyLength, float textWidthWebGL) {
  float ratio = textWidthWebGL > 0.0001 ? min(bodyLength / textWidthWebGL, 1.0) : 1.0;
  if (ratio < `.concat(t,`) return 0.0;
  if (ratio < `).concat(r,") return (ratio - ").concat(t,") / (").concat(r," - ").concat(t,`);
  return 1.0;
}
`)}var Lh=`
float computeEdgeLabelScreenDir(int pathId, float t, vec2 source, vec2 target, mat3 matrix) {
  vec2 tangent = queryPathTangent(pathId, t, source, target);
  return (matrix * vec3(tangent, 0.0)).x < 0.0 ? -1.0 : 1.0;
}
`,Ph=`
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
`;function as(e){var n=e.paths,t=e.minVisibilityThreshold,r=e.fullVisibilityThreshold,a=n.some(function(s){return s.hasSharpCorners}),i=n.map(function(s){return"// --- Path: ".concat(s.name,` ---
`).concat(s.glsl,`

// Tangent/normal functions: analytical if provided, otherwise numerical
`).concat(s.analyticalTangentGlsl||es(s.name),`

// Auto-generated fallbacks for any missing path functions
`).concat(ts(s.name,s.glsl),`

// Corner skip helpers (for paths with sharp corners like step/taxi)
`).concat(s.cornerSkipGlsl||"",`
`)}).join(`
`),o=a?`// Corner function selectors (only some paths have sharp corners)
vec2 queryGetCornerTs(int pathId, vec2 source, vec2 target) {
  switch (pathId) {
`.concat(n.map(function(s,l){return s.hasSharpCorners?"    case ".concat(l,": return path_").concat(s.name,"_getCornerTs(source, target);"):"    case ".concat(l,": return vec2(-1.0, -1.0); // No corners for ").concat(s.name)}).join(`
`),`
    default: return vec2(-1.0, -1.0);
  }
}

vec2 queryGetCornerConcavity(int pathId, vec2 source, vec2 target, float perpOffset) {
  switch (pathId) {
`).concat(n.map(function(s,l){return s.hasSharpCorners?"    case ".concat(l,": return path_").concat(s.name,"_getCornerConcavity(source, target, perpOffset);"):"    case ".concat(l,": return vec2(0.0, 0.0); // No corners for ").concat(s.name)}).join(`
`),`
    default: return vec2(0.0, 0.0);
  }
}`):"";return`
// ============================================================================
// Node data fetch (geometry texel) and per-edge clamp fetch (edge-frame texture)
// ============================================================================

`.concat(ct,`
`).concat(cn,`

// ============================================================================
// Path Functions (one block per path)
// ============================================================================

`).concat(i,`

// ============================================================================
// Path Query Selectors (dispatch by pathId)
// ============================================================================

`).concat(kn(n,"queryPathPosition","position","vec2","float t, vec2 source, vec2 target","t, source, target"),`

`).concat(kn(n,"queryPathTangent","tangent","vec2","float t, vec2 source, vec2 target","t, source, target"),`

`).concat(kn(n,"queryPathNormal","normal","vec2","float t, vec2 source, vec2 target","t, source, target"),`

`).concat(kn(n,"queryPathLength","length","float","vec2 source, vec2 target","source, target"),`

`).concat(kn(n,"queryPathTAtDistance","t_at_distance","float","float dist, vec2 source, vec2 target","dist, source, target"),`

`).concat(o,`

// ============================================================================
// Shared helpers (body bounds, alpha ramp, screen direction, perpendicular offset)
// ============================================================================

`).concat(Ch,`
`).concat(Fh(t,r),`
`).concat(Lh,`
`).concat(Ph,`
`)}var Ih=Ze.fontSize,Gh=new Map,is=24,Mo=(is+1)*2;function zh(e){var n=e.paths,t=e.fontSizeMode,r=e.headLengthRatio,a=e.tailLengthRatio,i=e.minVisibilityThreshold,o=e.fullVisibilityThreshold,s=e.keepUpright,l=t==="scaled",u=Ft(),d=Oe([].concat(K(n),[u])),c=Gn(d),g=`#version 300 es

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

`).concat(c.uniformDeclarations,`

out vec4 v_color;
out vec4 v_id;
out float v_alphaModifier;

const float ATLAS_FONT_SIZE = `).concat(ye(Ih),`;
const float HEAD_RATIO = `).concat(ye(r),`;
const float TAIL_RATIO = `).concat(ye(a),`;
const int RIBBON_SEGMENTS = `).concat(is,`;

// Path attribute varyings (declared as plain locals since this shader has no FS inputs for them)
`).concat(c.vertexVaryingDeclarations.replace(/out /g,""),`

// Node size variables used by some path functions (self-loops, etc.)
float v_sourceNodeSize;
float v_targetNodeSize;

// Shared preamble: shape SDFs, path functions + selectors, clamp, helpers.
`).concat(as({paths:n,minVisibilityThreshold:i,fullVisibilityThreshold:o}),`

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
`).concat(c.fetchCode,`
`).concat(c.varyingAssignments,`
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
`);return g}var Bh=`#version 300 es
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
`;function Nh(e,n,t,r){var a=r.shaderConfig,i=a.paths,o=a.fontSizeMode;if(i.length===0)throw new Error("createEdgeLabelBackgroundProgram: shaderConfig must declare at least one path");var s=Ft(),l=Oe([].concat(K(i),[s])),u=Ur([].concat(K(i),[s]),l),d=zh(a),c=(function(g){function h(f,y,v){var p;return ee(this,h),p=ge(this,h,[f,y,v]),D(p,"totalCount",0),D(p,"bufferCapacity",0),D(p,"edgeAttributeTexture",null),p.edgeAttributeTexture=new Wr(f,l),p.packedAttributeData=new Float32Array(l.floatsPerItem),p}return pe(h,g),te(h,[{key:"getDefinition",value:function(){for(var y=WebGL2RenderingContext,v=y.FLOAT,p=y.UNSIGNED_BYTE,m=y.TRIANGLE_STRIP,b=[],x=0;x<Mo;x++)b.push([x]);var _=["u_matrix","u_sizeRatio","u_correctionRatio","u_pixelRatio","u_cameraAngle","u_resolution","u_nodeDataTexture","u_nodeDataTextureWidth","u_edgeDataTexture","u_edgeDataTextureWidth","u_edgeFrameTexture","u_edgeFrameTextureWidth"];return o==="scaled"&&_.push("u_zoomSizeRatio"),l.floatsPerItem>0&&_.push("u_edgeAttributeTexture","u_edgeAttributeTextureWidth","u_edgeAttributeTexelsPerEdge"),{VERTICES:Mo,VERTEX_SHADER_SOURCE:d,FRAGMENT_SHADER_SOURCE:Bh,METHOD:m,UNIFORMS:_,ATTRIBUTES:[{name:"a_edgeIndex",size:1,type:v},{name:"a_edgeAttrIndex",size:1,type:v},{name:"a_baseFontSize",size:1,type:v},{name:"a_totalTextWidth",size:1,type:v},{name:"a_positionMode",size:1,type:v},{name:"a_margin",size:1,type:v},{name:"a_padding",size:1,type:v},{name:"a_color",size:4,type:p,normalized:!0},{name:"a_id",size:4,type:p,normalized:!0}],CONSTANT_ATTRIBUTES:[{name:"a_vertexIndex",size:1,type:v}],CONSTANT_DATA:b}}},{key:"processEdgeLabelBackground",value:function(y,v,p){var m=0;if(this.edgeAttributeTexture&&l.floatsPerItem>0){m=this.edgeAttributeTexture.allocate(v);var b=this.packedAttributeData;Or(u,p.edgeAttributes,b,"",Gh,0),this.edgeAttributeTexture.updateAllAttributes(v,b)}var x=this.floats,_=this.ints,T=y*this.STRIDE;x[T++]=p.edgeIndex,x[T++]=m,x[T++]=p.baseFontSize,x[T++]=p.totalTextWidth,x[T++]=p.positionMode,x[T++]=p.margin,x[T++]=p.padding,x[T++]=p.color,_[T++]=p.id}},{key:"setUniforms",value:function(y,v){var p=v.gl,m=v.uniformLocations;if(p.uniformMatrix3fv(m.u_matrix,!1,y.matrix),p.uniform1f(m.u_sizeRatio,y.sizeRatio),p.uniform1f(m.u_correctionRatio,y.correctionRatio),p.uniform1f(m.u_pixelRatio,y.pixelRatio),p.uniform1f(m.u_cameraAngle,y.cameraAngle),p.uniform2f(m.u_resolution,y.width,y.height),m.u_nodeDataTexture!==void 0&&p.uniform1i(m.u_nodeDataTexture,y.nodeDataTextureUnit),m.u_nodeDataTextureWidth!==void 0&&p.uniform1i(m.u_nodeDataTextureWidth,y.nodeDataTextureWidth),m.u_edgeDataTexture!==void 0&&p.uniform1i(m.u_edgeDataTexture,y.edgeDataTextureUnit),m.u_edgeDataTextureWidth!==void 0&&p.uniform1i(m.u_edgeDataTextureWidth,y.edgeDataTextureWidth),m.u_edgeFrameTexture!==void 0&&p.uniform1i(m.u_edgeFrameTexture,y.edgeFrameTextureUnit),m.u_edgeFrameTextureWidth!==void 0&&p.uniform1i(m.u_edgeFrameTextureWidth,y.edgeFrameTextureWidth),o==="scaled"&&m.u_zoomSizeRatio!==void 0){var b=this.renderer.getSetting("zoomToSizeRatioFunction");p.uniform1f(m.u_zoomSizeRatio,1/b(y.zoomRatio))}this.edgeAttributeTexture&&l.floatsPerItem>0&&m.u_edgeAttributeTexture!==void 0&&(this.edgeAttributeTexture.bind(bt),p.uniform1i(m.u_edgeAttributeTexture,bt),p.uniform1i(m.u_edgeAttributeTextureWidth,this.edgeAttributeTexture.getTextureWidth()),p.uniform1i(m.u_edgeAttributeTexelsPerEdge,this.edgeAttributeTexture.getTexelsPerItem()))}},{key:"renderProgram",value:function(y,v){this.edgeAttributeTexture&&l.floatsPerItem>0&&this.edgeAttributeTexture.upload(),Ee(h,"renderProgram",this,3)([y,v])}},{key:"kill",value:function(){this.edgeAttributeTexture&&(this.edgeAttributeTexture.kill(),this.edgeAttributeTexture=null),Ee(h,"kill",this,3)([])}},{key:"drawWebGL",value:function(y,v){var p=v.gl;this.totalCount!==0&&p.drawArraysInstanced(p.TRIANGLE_STRIP,0,this.VERTICES,this.totalCount)}},{key:"reallocate",value:function(y){this.totalCount=y,y>this.bufferCapacity&&(this.bufferCapacity=Math.max(y,Math.ceil(this.bufferCapacity*1.5)||10),Ee(h,"reallocate",this,3)([this.bufferCapacity]))}}])})(Gt);return new c(e,n,t)}function os(e){var n,t,r,a,i,o;return{paths:e.paths,headLengthRatio:(n=e.headLengthRatio)!==null&&n!==void 0?n:0,tailLengthRatio:(t=e.tailLengthRatio)!==null&&t!==void 0?t:0,fontSizeMode:(r=e.fontSizeMode)!==null&&r!==void 0?r:"fixed",minVisibilityThreshold:(a=e.minVisibilityThreshold)!==null&&a!==void 0?a:.7,fullVisibilityThreshold:(i=e.fullVisibilityThreshold)!==null&&i!==void 0?i:.8,keepUpright:(o=e.keepUpright)!==null&&o!==void 0?o:!0}}var Mh=Ze.fontSize,Wh=17/64;function Uh(e){var n=e.paths,t=e.hasBorder,r=t===void 0?!1:t,a=e.fontSizeMode,i=a===void 0?"fixed":a,o=e.minVisibilityThreshold,s=o===void 0?.5:o,l=e.fullVisibilityThreshold,u=l===void 0?.6:l,d=e.keepUpright,c=d===void 0?!0:d,g=i==="scaled",h=n.some(function(m){return m.hasSharpCorners}),f=Ft(),y=Oe([].concat(K(n),[f])),v=Gn(y),p=`#version 300 es

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
`).concat(g?"uniform float u_zoomSizeRatio;  // Zoom-based size ratio from zoomToSizeRatioFunction":"",`

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
const float ATLAS_FONT_SIZE = `).concat(ye(Mh),`;  // Base font size used in SDF atlas
const float VERTICAL_CENTER_RATIO = `).concat(ye(Wh),`;  // Baseline to visual center ratio

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
`).concat(as({paths:n,minVisibilityThreshold:s,fullVisibilityThreshold:u}),`

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
  `).concat(g?`// Scaled mode: font scales with zoom
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
  float readingSign = `).concat(c?"screenDir":"1.0",`;

  // Character center position relative to label center (on centerline, before curvature adjustment)
  float charCenterOffset = readingSign * (charOffsetWebGL + charAdvanceWebGL * 0.5 - textWidthWebGL * 0.5);

  // -------------------------------------------------------------------------
  // Step 6: Compute perpendicular offset based on position mode
  // -------------------------------------------------------------------------
  // Position modes: 0=over, 1=above, 2=below, 3=auto
  // Needed early for curvature-adaptive character spacing in Step 7
  float halfThickness = webGLThickness * 0.5;
  `).concat(g?`// Scaled mode: margin and text height scale with zoom (same factor as font)
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
    `.concat(g?"float skipDistGraph = STEP_INNER_CORNER_SKIP_FACTOR * baseFontSize * u_zoomSizeRatio * u_correctionRatio / u_sizeRatio;":"float skipDistGraph = STEP_INNER_CORNER_SKIP_FACTOR * baseFontSize * pixelToGraph;",`

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
  `).concat(g?"vec2 quadPosWebGL = quadPos * u_correctionRatio / u_sizeRatio; // Scaled mode":"vec2 quadPosWebGL = quadPos * pixelToGraph; // Fixed mode: use pixelToGraph for zoom-independent size",`

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
  `).concat(g?"float fadeWidthWebGL = FADE_WIDTH_PIXELS * u_correctionRatio / u_sizeRatio;":"float fadeWidthWebGL = FADE_WIDTH_PIXELS * pixelToGraph;",`

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
`);return p}function Oh(){var e=arguments.length>0&&arguments[0]!==void 0?arguments[0]:{},n=e.hasBorder,t=n===void 0?!1:n,r=`#version 300 es
precision highp float;

in vec2 v_texCoord;
in vec4 v_color;
`.concat(t?"in vec4 v_borderColor;":"",`
in float v_edgeFade;  // 0 = fully visible, 1 = fully faded
in float v_alphaModifier;  // 0-1 based on label visibility ratio
in float v_fontScale;  // Ratio of rendered font size to atlas font size
`).concat(t?"in float v_positionMode;  // Position mode (0=over, 1=above, 2=below, 3=auto)":"",`

uniform sampler2D u_atlas;
uniform float u_gamma;
uniform float u_sdfBuffer;
uniform float u_pixelRatio;
`).concat(t?"uniform float u_borderWidth;  // Border width in SDF units (normalized)":"",`

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

`).concat(t?`  // Fill alpha: fully opaque inside the glyph
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
`);return r}function Hh(e){var n=arguments.length>1&&arguments[1]!==void 0?arguments[1]:!1,t=arguments.length>2&&arguments[2]!==void 0?arguments[2]:"fixed",r=["u_matrix","u_sizeRatio","u_correctionRatio","u_pixelRatio","u_cameraAngle","u_sdfBufferPixels","u_resolution","u_atlasSize","u_atlas","u_gamma","u_sdfBuffer","u_nodeDataTexture","u_nodeDataTextureWidth","u_edgeDataTexture","u_edgeDataTextureWidth","u_edgeFrameTexture","u_edgeFrameTextureWidth","u_edgeAttributeTexture","u_edgeAttributeTextureWidth","u_edgeAttributeTexelsPerEdge"];t==="scaled"&&r.push("u_zoomSizeRatio"),n&&r.push("u_borderWidth");var a=U(e),i;try{for(a.s();!(i=a.n()).done;){var o=i.value,s=U(o.uniforms),l;try{for(s.s();!(l=s.n()).done;){var u=l.value;r.includes(u.name)||r.push(u.name)}}catch(d){s.e(d)}finally{s.f()}}}catch(d){a.e(d)}finally{a.f()}return r}function jh(e){var n=e.hasBorder,t=n===void 0?!1:n,r=e.fontSizeMode,a=r===void 0?"fixed":r;return{vertexShader:Uh(e),fragmentShader:Oh({hasBorder:t}),uniforms:Hh(e.paths,t,a)}}var $h=new Map;function Vh(e){switch(e){case"over":return 0;case"above":return 1;case"below":return 2;case"auto":return 3;default:return 0}}function qh(e,n,t,r){var a=r.color,i=r.margin,o=r.textBorder,s=os(r),l=s.paths,u=s.fontSizeMode,d=s.minVisibilityThreshold,c=s.fullVisibilityThreshold,g=s.keepUpright,h=!!o,f=jh({paths:l,hasBorder:h,fontSizeMode:u,minVisibilityThreshold:d,fullVisibilityThreshold:c,keepUpright:g}),y=(function(v){function p(m,b,x){var _;ee(this,p),_=ge(this,p,[m,b,x]),D(_,"atlasTexture",null),D(_,"atlasNeedsUpdate",!1),D(_,"refreshTimeout",null),D(_,"labelGlyphCache",new Map),D(_,"edgeAttributeTexture",null);var T=Ft();if(_.attributeLayout=Oe([].concat(K(l),[T])),_.attrDescriptors=Ur([].concat(K(l),[T]),_.attributeLayout),_.edgeAttributeTexture=new Wr(m,_.attributeLayout),_.packedAttributeData=new Float32Array(_.attributeLayout.floatsPerItem),_.atlasManager=new Pt,_.gamma=.025,_.sdfBuffer=Ze.cutoff,_.atlasTexture=m.createTexture(),!_.atlasTexture)throw new Error("EdgeLabelProgram: failed to create atlas texture");m.bindTexture(m.TEXTURE_2D,_.atlasTexture),m.texParameteri(m.TEXTURE_2D,m.TEXTURE_WRAP_S,m.CLAMP_TO_EDGE),m.texParameteri(m.TEXTURE_2D,m.TEXTURE_WRAP_T,m.CLAMP_TO_EDGE),m.texParameteri(m.TEXTURE_2D,m.TEXTURE_MIN_FILTER,m.LINEAR),m.texParameteri(m.TEXTURE_2D,m.TEXTURE_MAG_FILTER,m.LINEAR),m.bindTexture(m.TEXTURE_2D,null),_.atlasManager.on(Pt.ATLAS_UPDATED_EVENT,function(){_.atlasNeedsUpdate=!0,_.refreshTimeout!==null&&clearTimeout(_.refreshTimeout),_.refreshTimeout=setTimeout(function(){_.refreshTimeout=null,_.renderer.refresh()},0)});var R={family:"sans-serif",weight:"normal",style:"normal"};return _.defaultFontKey=_.atlasManager.registerFont(R),_}return pe(p,v),te(p,[{key:"getDefinition",value:function(){var b=WebGL2RenderingContext,x=b.FLOAT,_=b.UNSIGNED_BYTE,T=b.TRIANGLE_STRIP,R=new Set,A=[],S=U(l),k;try{for(S.s();!(k=S.n()).done;){var L=k.value,C=U(L.attributes),z;try{for(C.s();!(z=C.n()).done;){var F=z.value,P=F.name.startsWith("a_")?F.name:"a_".concat(F.name);R.has(P)||(R.add(P),A.push({name:P,size:F.size,type:F.type}))}}catch(N){C.e(N)}finally{C.f()}}}catch(N){S.e(N)}finally{S.f()}return{VERTICES:4,VERTEX_SHADER_SOURCE:f.vertexShader,FRAGMENT_SHADER_SOURCE:f.fragmentShader,METHOD:T,UNIFORMS:f.uniforms,ATTRIBUTES:[{name:"a_edgeIndex",size:1,type:x},{name:"a_edgeAttrIndex",size:1,type:x},{name:"a_baseFontSize",size:1,type:x},{name:"a_charMetrics",size:4,type:x},{name:"a_charDims",size:4,type:x},{name:"a_texCoords",size:4,type:x},{name:"a_labelParams",size:2,type:x},{name:"a_color",size:4,type:_,normalized:!0}].concat(K(h?[{name:"a_borderColor",size:4,type:_,normalized:!0}]:[]),K(A.filter(function(N){return!["a_curvature","curvature"].includes(N.name)}))),CONSTANT_ATTRIBUTES:[{name:"a_quadCorner",size:2,type:x}],CONSTANT_DATA:[[0,0],[1,0],[0,1],[1,1]]}}},{key:"prepareLabelGlyphs",value:function(b,x){if(x.hidden||!x.text){this.labelGlyphCache.delete(b);return}var _=x.text,T=x.fontKey||this.defaultFontKey;this.atlasManager.ensureGlyphs(_,T);var R=[],A=[],S=0,k=U(_),L;try{for(k.s();!(L=k.n()).done;){var C=L.value,z=C.codePointAt(0);if(z===void 0){R.push(void 0),A.push(S);continue}var F=this.atlasManager.getGlyph(z,T);R.push(F),A.push(S),F&&(S+=F.advance)}}catch(P){k.e(P)}finally{k.f()}this.labelGlyphCache.set(b,{glyphs:R,xOffsets:A,totalWidth:S})}},{key:"processCharacter",value:function(b,x,_,T){var R,A,S,k=this.floats,L=this.STRIDE,C=b*L,z=this.labelGlyphCache.get(x.parentKey);if(!z||!z.glyphs[T]){for(var F=0;F<L;F++)k[C+F]=0;return}var P=z.glyphs[T],N=z.xOffsets[T],G=p.labelColor,E=G&&ne(G)==="object"&&"color"in G&&G.color?G.color:x.color,I=We(E),W=C;k[W++]=x.edgeIndex,k[W++]=(R=(A=this.edgeAttributeTexture)===null||A===void 0?void 0:A.getIndex(x.parentKey))!==null&&R!==void 0?R:0,k[W++]=x.size,k[W++]=N,k[W++]=P.advance,k[W++]=z.totalWidth,k[W++]=Vh(x.position),k[W++]=P.atlasWidth,k[W++]=P.atlasHeight,k[W++]=P.bearingX,k[W++]=-P.bearingY,k[W++]=P.atlasX,k[W++]=P.atlasY,k[W++]=P.atlasWidth,k[W++]=P.atlasHeight;var B=(S=p.labelMargin)!==null&&S!==void 0?S:x.margin;if(k[W++]=B,k[W++]=0,k[W++]=I,h&&o){var w;typeof o.color=="string"?w=o.color:w=o.color.color||"#ffffff",k[W++]=We(w)}var $=new Set,j=U(l),J;try{for(j.s();!(J=j.n()).done;){var Se=J.value,le=U(Se.attributes),Y;try{for(le.s();!(Y=le.n()).done;){var Ke=Y.value,we=Ke.name.startsWith("a_")?Ke.name.slice(2):Ke.name;if(we!=="curvature"&&!$.has(we)){$.add(we);for(var At=0;At<Ke.size;At++)k[W++]=0}}}catch(je){le.e(je)}finally{le.f()}}}catch(je){j.e(je)}finally{j.f()}}},{key:"processEdgeLabel",value:function(b,x,_){if(this.prepareLabelGlyphs(b,_),this.edgeAttributeTexture&&!_.hidden&&_.text){this.edgeAttributeTexture.allocate(b);var T=this.packedAttributeData;Or(this.attrDescriptors,_.edgeAttributes,T,"",$h,0),this.edgeAttributeTexture.updateAllAttributes(b,T)}return Ee(p,"processLabel",this,3)([b,x,_])}},{key:"updateAtlasTexture",value:function(){if(this.atlasNeedsUpdate){var b=this.normalProgram.gl,x=this.atlasManager.getTextures();if(x.length!==0){var _=x[0];b.bindTexture(b.TEXTURE_2D,this.atlasTexture),b.texImage2D(b.TEXTURE_2D,0,b.RGBA,_.width,_.height,0,b.RGBA,b.UNSIGNED_BYTE,_.data),b.bindTexture(b.TEXTURE_2D,null),this.atlasNeedsUpdate=!1}}}},{key:"setUniforms",value:function(b,x){var _=x.gl,T=x.uniformLocations,R=this.atlasManager.getTextures(),A=R.length>0?[R[0].width,R[0].height]:[1,1];if(_.uniformMatrix3fv(T.u_matrix,!1,b.matrix),_.uniform1f(T.u_sizeRatio,b.sizeRatio),_.uniform1f(T.u_correctionRatio,b.correctionRatio),_.uniform1f(T.u_pixelRatio,b.pixelRatio),_.uniform1f(T.u_cameraAngle,b.cameraAngle),_.uniform1f(T.u_sdfBufferPixels,Ze.buffer),_.uniform2f(T.u_resolution,b.width,b.height),_.uniform2f(T.u_atlasSize,A[0],A[1]),_.uniform1f(T.u_gamma,this.gamma),_.uniform1f(T.u_sdfBuffer,this.sdfBuffer),_.activeTexture(_.TEXTURE0),_.bindTexture(_.TEXTURE_2D,this.atlasTexture),_.uniform1i(T.u_atlas,0),T.u_nodeDataTexture!==void 0&&_.uniform1i(T.u_nodeDataTexture,b.nodeDataTextureUnit),T.u_nodeDataTextureWidth!==void 0&&_.uniform1i(T.u_nodeDataTextureWidth,b.nodeDataTextureWidth),T.u_edgeDataTexture!==void 0&&_.uniform1i(T.u_edgeDataTexture,b.edgeDataTextureUnit),T.u_edgeDataTextureWidth!==void 0&&_.uniform1i(T.u_edgeDataTextureWidth,b.edgeDataTextureWidth),T.u_edgeFrameTexture!==void 0&&_.uniform1i(T.u_edgeFrameTexture,b.edgeFrameTextureUnit),T.u_edgeFrameTextureWidth!==void 0&&_.uniform1i(T.u_edgeFrameTextureWidth,b.edgeFrameTextureWidth),h&&o&&T.u_borderWidth!==void 0){var S=o.width/Ze.buffer*this.sdfBuffer;_.uniform1f(T.u_borderWidth,S)}if(this.edgeAttributeTexture&&T.u_edgeAttributeTexture!==void 0&&(this.edgeAttributeTexture.bind(bt),_.uniform1i(T.u_edgeAttributeTexture,bt),_.uniform1i(T.u_edgeAttributeTextureWidth,this.edgeAttributeTexture.getTextureWidth()),_.uniform1i(T.u_edgeAttributeTexelsPerEdge,this.edgeAttributeTexture.getTexelsPerItem())),u==="scaled"&&T.u_zoomSizeRatio!==void 0){var k=this.renderer.getSetting("zoomToSizeRatioFunction"),L=1/k(b.zoomRatio);_.uniform1f(T.u_zoomSizeRatio,L)}}},{key:"renderProgram",value:function(b,x){this.updateAtlasTexture(),this.atlasManager.hasPendingGlyphs()&&(this.atlasManager.flush(),this.updateAtlasTexture()),this.edgeAttributeTexture&&this.edgeAttributeTexture.upload(),Ee(p,"renderProgram",this,3)([b,x])}},{key:"registerFont",value:function(b){var x=arguments.length>1&&arguments[1]!==void 0?arguments[1]:"normal",_=arguments.length>2&&arguments[2]!==void 0?arguments[2]:"normal";return this.atlasManager.registerFont({family:b,weight:x,style:_})}},{key:"getAtlasManager",value:function(){return this.atlasManager}},{key:"ensureGlyphsReady",value:function(b,x){var _=x||this.defaultFontKey,T=U(b),R;try{for(T.s();!(R=T.n()).done;){var A=R.value;this.atlasManager.ensureGlyphs(A,_)}}catch(S){T.e(S)}finally{T.f()}this.atlasManager.flush()}},{key:"measureLabelAtlasWidth",value:function(b,x){var _=x||this.defaultFontKey;this.atlasManager.ensureGlyphs(b,_),this.atlasManager.hasPendingGlyphs()&&this.atlasManager.flush();var T=0,R=U(b),A;try{for(R.s();!(A=R.n()).done;){var S=A.value,k=S.codePointAt(0);if(k!==void 0){var L=this.atlasManager.getGlyph(k,_);L&&(T+=L.advance)}}}catch(C){R.e(C)}finally{R.f()}return T}},{key:"measureLabel",value:function(b,x,_){var T=this.measureLabelAtlasWidth(b,_),R=x/Ze.fontSize;return{width:T*R,height:x,textHeight:x}}},{key:"kill",value:function(){var b=this.normalProgram.gl;this.refreshTimeout!==null&&(clearTimeout(this.refreshTimeout),this.refreshTimeout=null),this.atlasTexture&&(b.deleteTexture(this.atlasTexture),this.atlasTexture=null),this.edgeAttributeTexture&&(this.edgeAttributeTexture.kill(),this.edgeAttributeTexture=null),this.atlasManager.destroy(),this.labelGlyphCache.clear(),Ee(p,"kill",this,3)([])}}])})(Ko);return D(y,"labelColor",a),D(y,"labelMargin",i),new y(e,n,t)}function Xh(){var e=`
// No extremity - always returns positive (outside)
float extremity_none(vec2 uv, float lengthRatio, float widthRatio) {
  return 1.0;
}
`;return{name:"none",glsl:e,length:0,widthFactor:1,margin:0,uniforms:[],attributes:[]}}function ss(e,n,t,r,a,i,o){var s,l,u=ns(r),d=u.paths,c=u.layers,g=u.defaultHead,h=u.defaultTail,f=[Xh()].concat(K(u.extremities)),y={},v={};d.forEach(function(C,z){return y[C.name]=z}),f.forEach(function(C,z){return v[C.name]=z});var p=(s=v[g])!==null&&s!==void 0?s:0,m=(l=v[h])!==null&&l!==void 0?l:0,b=null,x=Oe([].concat(K(d),K(c))),_=(function(C){function z(F,P,N){var G;ee(this,z),b||(b=No({paths:d,extremities:f,layers:c,antialias:a})),G=ge(this,z,[F,P,N]),D(G,"layerLifecycles",new Map),D(G,"needsShaderRegeneration",!1),D(G,"edgeAttributeTexture",null),D(G,"layout",x),D(G,"attrDescriptors",[]),D(G,"lifecycleIndexOffset",d.length),G._pickingBuffer=P,G.edgeAttributeTexture=new Wr(F,G.layout),G.packedAttributeData=new Float32Array(G.layout.floatsPerItem),c.forEach(function(I,W){if(I.lifecycle){var B={gl:F,renderer:{refresh:function(){return N.refresh()}},getUniformLocation:function($){return F.getUniformLocation(G.normalProgram.program,$)},requestShaderRegeneration:function(){G.needsShaderRegeneration=!0},requestRefresh:function(){N.refresh()}};G.layerLifecycles.set(W,I.lifecycle(B))}}),G.layerLifecycles.forEach(function(I){var W;return(W=I.init)===null||W===void 0?void 0:W.call(I)});var E=new Map;return G.layerLifecycles.forEach(function(I,W){I.getAttributeData&&E.set(d.length+W,I)}),G.attrDescriptors=Ur([].concat(K(d),K(c)),G.layout,E),G}return pe(z,C),te(z,[{key:"getAttributeTexture",value:function(){return this.edgeAttributeTexture}},{key:"resolveEdgeIds",value:function(P,N,G){var E=P,I=0,W=p,B=m,w=N?E.selfLoopPath:G&&E.parallelPath?E.parallelPath:E.path;w&&y[w]!==void 0&&(I=y[w]),E.head&&E.head!=="none"&&v[E.head]!==void 0&&(W=v[E.head]),E.tail&&E.tail!=="none"&&v[E.tail]!==void 0&&(B=v[E.tail]);var $=f[W],j=f[B];return{pathId:I,headId:W,tailId:B,headLengthRatio:lt($.length)?0:$.length,tailLengthRatio:lt(j.length)?0:j.length}}},{key:"getDefinition",value:function(){var P=WebGL2RenderingContext,N=P.TRIANGLE_STRIP,G=N,E=b;return{VERTICES:E.verticesPerEdge,VERTEX_SHADER_SOURCE:E.vertexShader,FRAGMENT_SHADER_SOURCE:E.fragmentShader,METHOD:G,UNIFORMS:E.uniforms,ATTRIBUTES:E.attributes,CONSTANT_ATTRIBUTES:E.constantAttributes,CONSTANT_DATA:E.constantData}}},{key:"maybeRegenerateShaders",value:function(){var P=this;if(this.needsShaderRegeneration){this.needsShaderRegeneration=!1;var N=c.map(function($,j){var J=P.layerLifecycles.get(j);return J!=null&&J.regenerate?J.regenerate():$});b=No({paths:d,extremities:f,layers:N,antialias:this.renderer.getSetting("antialiasEdges")});var G=this.normalProgram.gl,E=this.normalProgram,I=E.program,W=E.buffer,B=E.vertexShader,w=E.fragmentShader;G.deleteProgram(I),G.deleteBuffer(W),G.deleteShader(B),G.deleteShader(w),this.normalProgram=this.getProgramInfo("normal",G,b.vertexShader,b.fragmentShader,this._pickingBuffer)}}},{key:"process",value:function(P,N,G,E,I,W){var B=N*this.STRIDE;if(I.visibility==="hidden"||G.visibility==="hidden"||E.visibility==="hidden"){for(var w=B+this.STRIDE;B<w;B++)this.floats[B]=0;this.floats[N*this.STRIDE]=Xo;return}this.processVisibleItem(en(P),B,G,E,I,W)}},{key:"processVisibleItem",value:function(P,N,G,E,I,W){var B,w=this.floats,$=this.ints;w[N++]=W,w[N++]=We(I.color),$[N++]=P,w[N++]=(B=I.opacity)!==null&&B!==void 0?B:1;var j=this.packedAttributeData;Or(this.attrDescriptors,I,j,I.color,this.layerLifecycles,this.lifecycleIndexOffset),this.edgeAttributeTexture.updateAllAttributesAtRow(W,j)}},{key:"setUniforms",value:function(P,N){var G=this,E=N.gl,I=N.uniformLocations;I.u_matrix&&E.uniformMatrix3fv(I.u_matrix,!1,P.matrix),I.u_sizeRatio&&E.uniform1f(I.u_sizeRatio,P.sizeRatio),I.u_correctionRatio&&E.uniform1f(I.u_correctionRatio,P.correctionRatio),I.u_zoomRatio&&E.uniform1f(I.u_zoomRatio,P.zoomRatio),I.u_pixelRatio&&E.uniform1f(I.u_pixelRatio,P.pixelRatio),I.u_cameraAngle&&E.uniform1f(I.u_cameraAngle,P.cameraAngle),I.u_feather&&E.uniform1f(I.u_feather,P.antiAliasingFeather),I.u_minEdgeThickness&&E.uniform1f(I.u_minEdgeThickness,P.minEdgeThickness),I.u_pickingPadding&&E.uniform1f(I.u_pickingPadding,P.edgePickingPadding),I.u_nodeDataTexture&&E.uniform1i(I.u_nodeDataTexture,P.nodeDataTextureUnit),I.u_nodeDataTextureWidth&&E.uniform1i(I.u_nodeDataTextureWidth,P.nodeDataTextureWidth),I.u_edgeDataTexture&&E.uniform1i(I.u_edgeDataTexture,P.edgeDataTextureUnit),I.u_edgeDataTextureWidth&&E.uniform1i(I.u_edgeDataTextureWidth,P.edgeDataTextureWidth),I.u_edgeFrameTexture&&E.uniform1i(I.u_edgeFrameTexture,P.edgeFrameTextureUnit),I.u_edgeFrameTextureWidth&&E.uniform1i(I.u_edgeFrameTextureWidth,P.edgeFrameTextureWidth),this.edgeAttributeTexture&&this.layout.floatsPerItem>0&&(this.edgeAttributeTexture.bind(bt),I.u_edgeAttributeTexture&&E.uniform1i(I.u_edgeAttributeTexture,bt),I.u_edgeAttributeTextureWidth&&E.uniform1i(I.u_edgeAttributeTextureWidth,this.edgeAttributeTexture.getTextureWidth()),I.u_edgeAttributeTexelsPerEdge&&E.uniform1i(I.u_edgeAttributeTexelsPerEdge,this.edgeAttributeTexture.getTexelsPerItem()));var W=new Set;d.forEach(function(B){B.uniforms.forEach(function(w){W.has(w.name)||(W.add(w.name),G.setTypedUniform(w,N))})}),f.forEach(function(B){B.uniforms.forEach(function(w){W.has(w.name)||(W.add(w.name),G.setTypedUniform(w,N))})}),c.forEach(function(B){B.uniforms.forEach(function(w){W.has(w.name)||(W.add(w.name),G.setTypedUniform(w,N))})})}},{key:"render",value:function(P,N,G){this.maybeRegenerateShaders(),Ee(z,"render",this,3)([P,N,G])}},{key:"renderProgram",value:function(P,N){this.layerLifecycles.forEach(function(G){var E;return(E=G.beforeRender)===null||E===void 0?void 0:E.call(G)}),Ee(z,"renderProgram",this,3)([P,N])}},{key:"uploadAttributeTexture",value:function(){this.edgeAttributeTexture&&this.layout.floatsPerItem>0&&this.edgeAttributeTexture.upload()}},{key:"kill",value:function(){this.layerLifecycles.forEach(function(P){var N;return(N=P.kill)===null||N===void 0?void 0:N.call(P)}),this.edgeAttributeTexture&&(this.edgeAttributeTexture.kill(),this.edgeAttributeTexture=null),Ee(z,"kill",this,3)([])}}])})(Gt),T=f[p],R=f[m],A=O({paths:d,headLengthRatio:lt(T.length)?0:T.length,tailLengthRatio:lt(R.length)?0:R.length},r.label),S=os(A),k=function(){return{labelProgram:qh(e,null,t,A),labelBackgroundProgram:Nh(e,n,t,{shaderConfig:S})}},L=new kh(e,{paths:d,extremities:f,layers:c,nodeShapes:i,nodeLayers:o});return{edgeProgram:new _(e,null,t),createLabelPrograms:k,framePass:L}}function Yh(e){return ne(e)==="object"&&"uniforms"in e&&Array.isArray(e.uniforms)}function Jh(e){return ne(e)==="object"&&"uniforms"in e&&"attributes"in e&&"glsl"in e}function Kh(e){return ne(e)==="object"&&"uniforms"in e&&"attributes"in e&&"segments"in e}function Zh(e){return ne(e)==="object"&&"uniforms"in e&&"attributes"in e&&"length"in e}function Qh(e){return ne(e)==="object"&&"uniforms"in e&&"attributes"in e&&"glsl"in e}function ef(e){return ne(e)==="object"&&"glsl"in e&&"name"in e&&!("uniforms"in e)}function tf(e){return ne(e)==="object"&&"glsl"in e&&"name"in e&&!("uniforms"in e)}function nf(e){return ne(e)==="object"&&"glsl"in e&&"name"in e&&!("uniforms"in e)}function rf(e){if(Yh(e))return e;if(ef(e))return{name:e.name,glsl:e.glsl,inradiusFactor:e.inradiusFactor,uniforms:[]};throw new Error("Invalid node shape specification: ".concat(JSON.stringify(e)))}function af(e){if(Jh(e))return e;if(lo(e))return{name:e.name,glsl:e.glsl,uniforms:[],attributes:[]};throw new Error("Invalid node layer specification: ".concat(JSON.stringify(e)))}function of(e){if(Kh(e))return e;if(tf(e))return{name:e.name,glsl:e.glsl,segments:e.segments,uniforms:[],attributes:[]};throw new Error("Invalid edge path specification: ".concat(JSON.stringify(e)))}function sf(e){if(Qh(e))return e;if(uo(e))return{name:e.name,glsl:e.glsl,uniforms:[],attributes:[]};throw new Error("Invalid edge layer specification: ".concat(JSON.stringify(e)))}function lf(e){if(Zh(e))return e;if(nf(e))return{name:e.name,glsl:e.glsl,length:e.length,widthFactor:e.widthFactor,margin:0,uniforms:[],attributes:[]};throw new Error("Invalid edge extremity specification: ".concat(JSON.stringify(e)))}function ls(e){var n,t,r=(n=e?.shapes)!==null&&n!==void 0?n:An.shapes,a=(t=e?.layers)!==null&&t!==void 0?t:An.layers,i=r.map(rf),o=a.map(af);if(i.length===0)throw new Error("At least one node shape must be specified.");if(o.length===0)throw new Error("At least one node layer must be specified.");return{shapes:i,layers:o}}function uf(e){var n,t,r,a=(n=e?.paths)!==null&&n!==void 0?n:tn.paths,i=(t=e?.extremities)!==null&&t!==void 0?t:tn.extremities,o=(r=e?.layers)!==null&&r!==void 0?r:tn.layers,s=a.map(of),l=i.map(lf).filter(function(d){return d!==null}),u=o.map(sf);if(s.length===0)throw new Error("At least one edge path must be specified.");if(u.length===0)throw new Error("At least one edge layer must be specified.");return{paths:s,extremities:l,layers:u}}function us(e,n){var t={},r=U(e),a;try{for(r.s();!(a=r.n()).done;){var i=a.value;i.variables&&Object.assign(t,i.variables)}}catch(o){r.e(o)}finally{r.f()}return Object.assign(t,n||{}),t}function cs(e,n,t,r,a){var i=ls(r),o=i.shapes,s=i.layers,l=us(o,r?.variables),u=Qo(e,n,t,{shapes:o,layers:s,label:r?.label,backdrop:r?.backdrop,labelAttachments:r?.labelAttachments},a);return O(O({},u),{},{variables:l})}function ds(e,n,t,r,a,i){var o=uf(r),s=o.paths,l=o.extremities,u=o.layers,d=ls(i),c=d.shapes,g=d.layers,h=us(s,r?.variables),f=ss(e,n,t,{paths:s,extremities:l,layers:u,defaultHead:r?.defaultHead,defaultTail:r?.defaultTail,label:r?.label},a,c,g);return O(O({},f),{},{variables:h,paths:s})}var cf=2;function hs(e,n,t){var r=arguments.length>3&&arguments[3]!==void 0?arguments[3]:cf,a=n.canvas,i=a.width,o=a.height,s=t.x,l=t.y,u=t.rowHeight,d=t.maxRowWidth,c={},g=[],h=U(e),f;try{for(h.s();!(f=h.n()).done;){var y=f.value,v=y.width+r,p=y.height+r;if(v>i||p>o||s+v>i&&l+u+p>o){g.push(y);continue}s+v>i&&(d=Math.max(d,s),s=0,l+=u,u=p),y.draw(n,s,l),c[y.key]={x:s,y:l,width:y.width,height:y.height},s+=v,u=Math.max(u,p)}}catch(m){h.e(m)}finally{h.f()}return d=Math.max(d,s),{atlas:c,cursor:{x:s,y:l,rowHeight:u,maxRowWidth:d},remaining:g}}function Pe(e,n,t,r){var a=Object.defineProperty;try{a({},"",{})}catch{a=0}Pe=function(i,o,s,l){function u(d,c){Pe(i,d,function(g){return this._invoke(d,c,g)})}o?a?a(i,o,{value:s,enumerable:!l,configurable:!l,writable:!l}):i[o]=s:(u("next",0),u("throw",1),u("return",2))},Pe(e,n,t,r)}function Ie(){var e,n,t=typeof Symbol=="function"?Symbol:{},r=t.iterator||"@@iterator",a=t.toStringTag||"@@toStringTag";function i(h,f,y,v){var p=f&&f.prototype instanceof s?f:s,m=Object.create(p.prototype);return Pe(m,"_invoke",(function(b,x,_){var T,R,A,S=0,k=_||[],L=!1,C={p:0,n:0,v:e,a:z,f:z.bind(e,4),d:function(F,P){return T=F,R=0,A=e,C.n=P,o}};function z(F,P){for(R=F,A=P,n=0;!L&&S&&!N&&n<k.length;n++){var N,G=k[n],E=C.p,I=G[2];F>3?(N=I===P)&&(A=G[(R=G[4])?5:(R=3,3)],G[4]=G[5]=e):G[0]<=E&&((N=F<2&&E<G[1])?(R=0,C.v=P,C.n=G[1]):E<I&&(N=F<3||G[0]>P||P>I)&&(G[4]=F,G[5]=P,C.n=I,R=0))}if(N||F>1)return o;throw L=!0,P}return function(F,P,N){if(S>1)throw TypeError("Generator is already running");for(L&&P===1&&z(P,N),R=P,A=N;(n=R<2?e:A)||!L;){T||(R?R<3?(R>1&&(C.n=-1),z(R,A)):C.n=A:C.v=A);try{if(S=2,T){if(R||(F="next"),n=T[F]){if(!(n=n.call(T,A)))throw TypeError("iterator result is not an object");if(!n.done)return n;A=n.value,R<2&&(R=0)}else R===1&&(n=T.return)&&n.call(T),R<2&&(A=TypeError("The iterator does not provide a '"+F+"' method"),R=1);T=e}else if((n=(L=C.n<0)?A:b.call(x,C))!==o)break}catch(G){T=e,R=1,A=G}finally{S=1}}return{value:n,done:L}}})(h,y,v),!0),m}var o={};function s(){}function l(){}function u(){}n=Object.getPrototypeOf;var d=[][r]?n(n([][r]())):(Pe(n={},r,function(){return this}),n),c=u.prototype=s.prototype=Object.create(d);function g(h){return Object.setPrototypeOf?Object.setPrototypeOf(h,u):(h.__proto__=u,Pe(h,a,"GeneratorFunction")),h.prototype=Object.create(c),h}return l.prototype=u,Pe(c,"constructor",u),Pe(u,"constructor",l),l.displayName="GeneratorFunction",Pe(u,a,"GeneratorFunction"),Pe(c),Pe(c,a,"Generator"),Pe(c,r,function(){return this}),Pe(c,"toString",function(){return"[object Generator]"}),(Ie=function(){return{w:i,m:g}})()}function fs(e,n,t,r,a,i,o){try{var s=e[i](o),l=s.value}catch(u){return void t(u)}s.done?n(l):Promise.resolve(l).then(r,a)}function fn(e){return function(){var n=this,t=arguments;return new Promise(function(r,a){var i=e.apply(n,t);function o(l){fs(i,r,a,o,s,"next",l)}function s(l){fs(i,r,a,o,s,"throw",l)}o(void 0)})}}var jr=new Map,gs=!1;function df(e){return new Promise(function(n,t){var r=new FileReader;r.onload=function(){return n(r.result)},r.onerror=t,r.readAsDataURL(e)})}function hf(e){if(!jr.has(e)){var n=fetch(e).then(function(t){return t.blob()}).then(df).catch(function(){return jr.delete(e),null});jr.set(e,n)}return jr.get(e)}function ff(e){return Oa.apply(this,arguments)}function Oa(){return Oa=fn(Ie().m(function e(n){var t,r,a;return Ie().w(function(i){for(;;)switch(i.n){case 0:return t=document.createElement("div"),t.innerHTML=n,r=Array.from(t.querySelectorAll("img[src]")),a=r.filter(function(o){return!o.getAttribute("src").startsWith("data:")}),i.n=1,Promise.all(a.map((function(){var o=fn(Ie().m(function s(l){var u;return Ie().w(function(d){for(;;)switch(d.n){case 0:return d.n=1,hf(l.getAttribute("src"));case 1:u=d.v,u&&l.setAttribute("src",u);case 2:return d.a(2)}},s)}));return function(s){return o.apply(this,arguments)}})()));case 1:return i.a(2,t.innerHTML)}},e)})),Oa.apply(this,arguments)}function ps(e){return Ha.apply(this,arguments)}function Ha(){return Ha=fn(Ie().m(function e(n){var t,r,a,i,o,s,l,u,d,c,g,h,f,y=arguments;return Ie().w(function(v){for(;;)switch(v.n){case 0:if(t=y.length>1&&y[1]!==void 0?y[1]:1,r=n instanceof SVGElement?new XMLSerializer().serializeToString(n):n,a=new DOMParser,i=a.parseFromString(r,"image/svg+xml"),o=i.querySelector("svg"),o){v.n=1;break}return v.a(2,null);case 1:if(s=parseFloat(o.getAttribute("width")||""),l=parseFloat(o.getAttribute("height")||""),(!(s>0)||!(l>0))&&(u=o.getAttribute("viewBox"),u&&(d=u.trim().split(/[\s,]+/),s=parseFloat(d[2]),l=parseFloat(d[3]))),!(!(s>0)||!(l>0))){v.n=2;break}return console.warn("Sigma: SVG label attachment has no parseable dimensions \u2014 skipped."),v.a(2,null);case 2:return c=Math.ceil(s*t),g=Math.ceil(l*t),h=new Blob([r],{type:"image/svg+xml"}),f=URL.createObjectURL(h),v.a(2,new Promise(function(p){var m=new Image;m.onload=function(){var b=document.createElement("canvas");b.width=c,b.height=g,b.getContext("2d").drawImage(m,0,0,c,g),URL.revokeObjectURL(f);try{b.getContext("2d").getImageData(0,0,1,1)}catch(x){if(x instanceof DOMException&&x.name==="SecurityError"){gs||(gs=!0,console.warn('Sigma: A label attachment was skipped because the rendered canvas is tainted. SVG with <foreignObject> (used by the "html" attachment type) is blocked in Chromium and Safari. Use type: "canvas" with Canvas 2D rendering instead.')),p(null);return}throw x}p(b)},m.onerror=function(){URL.revokeObjectURL(f),p(null)},m.src=f}))}},e)})),Ha.apply(this,arguments)}function gf(e,n){var t=document.createElement("div");t.style.cssText="position:fixed;left:-99999px;top:0;visibility:hidden;width:max-content;height:max-content",t.innerHTML=(n?"<style>".concat(n,"</style>"):"")+e,document.body.appendChild(t);var r=t.getBoundingClientRect(),a=r.width,i=r.height;return document.body.removeChild(t),{width:Math.ceil(a),height:Math.ceil(i)}}function pf(e,n,t,r){return ja.apply(this,arguments)}function ja(){return ja=fn(Ie().m(function e(n,t,r,a){var i,o,s,l,u,d,c,g,h=arguments;return Ie().w(function(f){for(;;)switch(f.n){case 0:return i=h.length>4&&h[4]!==void 0?h[4]:1,o=n instanceof HTMLElement?n.outerHTML:n,f.n=1,ff(o);case 1:return s=f.v,(r==null||a==null)&&(l=gf(s,t),r=r??l.width,a=a??l.height),u=Math.ceil(r*i),d=Math.ceil(a*i),c=t?"<style>".concat(t,"</style>"):"",g='<svg xmlns="http://www.w3.org/2000/svg" '+'width="'.concat(u,'" height="').concat(d,'" viewBox="0 0 ').concat(r," ").concat(a,'">')+'<foreignObject width="'.concat(r,'" height="').concat(a,'">')+'<body xmlns="http://www.w3.org/1999/xhtml" style="margin:0;padding:0">'.concat(c).concat(s,"</body>")+"</foreignObject></svg>",f.a(2,ps(g,1))}},e)})),ja.apply(this,arguments)}function vf(e){return $a.apply(this,arguments)}function $a(){return $a=fn(Ie().m(function e(n){var t,r=arguments,a;return Ie().w(function(i){for(;;)switch(i.n){case 0:t=r.length>1&&r[1]!==void 0?r[1]:1,a=n.type,i.n=a==="canvas"?1:a==="svg"?2:a==="html"?3:4;break;case 1:return i.a(2,n.canvas);case 2:return i.a(2,ps(n.svg,t));case 3:return i.a(2,pf(n.html,n.css,n.width,n.height,t));case 4:return i.a(2)}},e)})),$a.apply(this,arguments)}var hn=2048,vs=(function(){function e(n,t,r){ee(this,e),D(this,"cache",new Map),D(this,"pending",new Set),D(this,"atlas",{}),D(this,"glTexture",null),D(this,"dirty",!0),this.gl=n,this.renderers=t,this.scheduleRender=r,this.packCanvas=document.createElement("canvas"),this.packCanvas.width=hn,this.packCanvas.height=hn,this.packCtx=this.packCanvas.getContext("2d")}return te(e,[{key:"renderAttachment",value:function(t,r,a){var i=this,o="".concat(t,":").concat(r);if(!(this.cache.has(o)||this.pending.has(o))){var s=this.renderers[r];if(s){var l=s(a);if(l){this.pending.add(o);var u=a.pixelRatio;Promise.resolve(l).then((function(){var d=fn(Ie().m(function c(g){var h;return Ie().w(function(f){for(;;)switch(f.n){case 0:if(i.pending.has(o)){f.n=1;break}return f.a(2);case 1:if(i.pending.delete(o),g){f.n=2;break}return f.a(2);case 2:return f.n=3,vf(g,u);case 3:if(h=f.v,!(!h||h.width===0||h.height===0)){f.n=4;break}return f.a(2);case 4:i.cache.set(o,{image:h,width:h.width,height:h.height}),i.dirty=!0,i.scheduleRender();case 5:return f.a(2)}},c)}));return function(c){return d.apply(this,arguments)}})())}}}}},{key:"regenerateAtlas",value:function(){if(this.dirty){this.dirty=!1;var t=[];if(this.cache.forEach(function(u,d){t.push({key:d,width:u.width,height:u.height,draw:function(g,h,f){g.drawImage(u.image,h,f)}})}),t.length===0){this.atlas={},this.deleteGLTexture();return}var r={x:0,y:0,rowHeight:0,maxRowWidth:0};this.packCtx.clearRect(0,0,hn,hn);var a=hs(t,this.packCtx,r),i=a.atlas,o=a.remaining;this.atlas=i,o.length>0&&console.warn("Sigma: ".concat(o.length," label attachment(s) could not fit in the ").concat(hn,"x").concat(hn," atlas and will not be rendered.")),this.deleteGLTexture();var s=this.gl,l=s.createTexture();s.activeTexture(s.TEXTURE0+In),s.bindTexture(s.TEXTURE_2D,l),s.pixelStorei(s.UNPACK_PREMULTIPLY_ALPHA_WEBGL,!0),s.texImage2D(s.TEXTURE_2D,0,s.RGBA,s.RGBA,s.UNSIGNED_BYTE,this.packCanvas),s.pixelStorei(s.UNPACK_PREMULTIPLY_ALPHA_WEBGL,!1),s.texParameteri(s.TEXTURE_2D,s.TEXTURE_MIN_FILTER,s.LINEAR),s.texParameteri(s.TEXTURE_2D,s.TEXTURE_MAG_FILTER,s.LINEAR),s.texParameteri(s.TEXTURE_2D,s.TEXTURE_WRAP_S,s.CLAMP_TO_EDGE),s.texParameteri(s.TEXTURE_2D,s.TEXTURE_WRAP_T,s.CLAMP_TO_EDGE),this.glTexture=l}}},{key:"bindTexture",value:function(t){if(this.glTexture){var r=this.gl;r.activeTexture(r.TEXTURE0+t),r.bindTexture(r.TEXTURE_2D,this.glTexture)}}},{key:"getEntry",value:function(t,r){var a="".concat(t,":").concat(r);return this.atlas[a]||null}},{key:"invalidateNode",value:function(t){var r="".concat(t,":"),a=U(this.cache.keys()),i;try{for(a.s();!(i=a.n()).done;){var o=i.value;o.startsWith(r)&&(this.cache.delete(o),this.dirty=!0)}}catch(d){a.e(d)}finally{a.f()}var s=U(this.pending),l;try{for(s.s();!(l=s.n()).done;){var u=l.value;u.startsWith(r)&&this.pending.delete(u)}}catch(d){s.e(d)}finally{s.f()}}},{key:"restore",value:function(){this.glTexture=null,this.dirty=!0}},{key:"clear",value:function(){this.cache.clear(),this.pending.clear(),this.atlas={},this.dirty=!0,this.deleteGLTexture()}},{key:"kill",value:function(){this.clear(),this.packCanvas=null,this.packCtx=null,this.gl=null}},{key:"deleteGLTexture",value:function(){this.glTexture&&(this.gl.deleteTexture(this.glTexture),this.glTexture=null)}}])})(),Va=(function(){function e(n){ee(this,e),D(this,"buckets",new Map),D(this,"keyDepth",new Map);var t=U(n),r;try{for(t.s();!(r=t.n()).done;){var a=r.value;this.buckets.set(a,new Set)}}catch(i){t.e(i)}finally{t.f()}}return te(e,[{key:"has",value:function(t){return this.keyDepth.has(t)}},{key:"getBucket",value:function(t){return this.buckets.get(t)}},{key:"set",value:function(t,r){var a,i=this.keyDepth.get(t);if(i!==r){var o=this.buckets.get(r);if(!o)throw new Error('Sigma: "'.concat(r,'" is not a declared depth layer'));i!==void 0&&((a=this.buckets.get(i))===null||a===void 0||a.delete(t)),o.add(t),this.keyDepth.set(t,r)}}},{key:"remove",value:function(t){var r=this.keyDepth.get(t);r!==void 0&&(this.buckets.get(r).delete(t),this.keyDepth.delete(t))}},{key:"clearAll",value:function(){var t=U(this.buckets.values()),r;try{for(t.s();!(r=t.n()).done;){var a=r.value;a.clear()}}catch(i){t.e(i)}finally{t.f()}this.keyDepth.clear()}},{key:"getSorted",value:function(t,r){var a=this.buckets.get(t);return!a||a.size===0?[]:K(a).sort(function(i,o){return r(i)-r(o)})}}])})(),ms=(function(e){function n(t,r){return ee(this,n),ge(this,n,[t,2,r])}return pe(n,e),te(n,[{key:"updateNode",value:function(r,a,i,o,s,l,u,d){var c=this.indexMap.get(r);if(c===void 0)throw new Error('Node "'.concat(r,'" not allocated in NodeDataTexture'));var g=pt(d),h=re(g,4),f=h[0],y=h[1],v=h[2],p=h[3],m=c*2*4;this.data[m]=a,this.data[m+1]=i,this.data[m+2]=o,this.data[m+3]=s,this.data[m+4]=l,this.data[m+5]=u,this.data[m+6]=f*65536+y*256+v,this.data[m+7]=p/255,this.markDirty(c)}}])})(Mr),mf=1024,bf=1.5,yf=4096,qa=(function(){function e(n,t){var r;if(ee(this,e),D(this,"texture",null),D(this,"framebuffer",null),!n.getExtension("EXT_color_buffer_float"))throw new Error("sigma: EXT_color_buffer_float is required for label/edge placement but is unavailable in this WebGL2 context.");this.gl=n,this.channels=t.channels,this.capacity=this.roundUpToPowerOfTwo((r=t.initialCapacity)!==null&&r!==void 0?r:mf);var a=this.computeDimensions(this.capacity);this.textureWidth=a.width,this.textureHeight=a.height,this.create()}return te(e,[{key:"roundUpToPowerOfTwo",value:function(t){return Math.pow(2,Math.ceil(Math.log2(Math.max(1,t))))}},{key:"computeDimensions",value:function(t){var r=Math.min(t,yf),a=Math.ceil(t/r);return{width:r,height:a}}},{key:"create",value:function(){var t=this.gl,r=this.channels===1?t.R32F:t.RGBA32F,a=this.channels===1?t.RED:t.RGBA;if(this.texture=t.createTexture(),t.activeTexture(t.TEXTURE0),t.bindTexture(t.TEXTURE_2D,this.texture),t.texImage2D(t.TEXTURE_2D,0,r,this.textureWidth,this.textureHeight,0,a,t.FLOAT,null),t.texParameteri(t.TEXTURE_2D,t.TEXTURE_MIN_FILTER,t.NEAREST),t.texParameteri(t.TEXTURE_2D,t.TEXTURE_MAG_FILTER,t.NEAREST),t.texParameteri(t.TEXTURE_2D,t.TEXTURE_WRAP_S,t.CLAMP_TO_EDGE),t.texParameteri(t.TEXTURE_2D,t.TEXTURE_WRAP_T,t.CLAMP_TO_EDGE),this.framebuffer=t.createFramebuffer(),t.bindFramebuffer(t.FRAMEBUFFER,this.framebuffer),t.framebufferTexture2D(t.FRAMEBUFFER,t.COLOR_ATTACHMENT0,t.TEXTURE_2D,this.texture,0),t.checkFramebufferStatus(t.FRAMEBUFFER)!==t.FRAMEBUFFER_COMPLETE)throw new Error("sigma: float framebuffer for frame-pass placement is incomplete.");t.bindFramebuffer(t.FRAMEBUFFER,null),t.bindTexture(t.TEXTURE_2D,null)}},{key:"ensureCapacity",value:function(t){if(!(t<=this.capacity)){var r=this.gl;this.capacity=this.roundUpToPowerOfTwo(Math.ceil(t*bf));var a=this.computeDimensions(this.capacity);this.textureWidth=a.width,this.textureHeight=a.height,this.texture&&r.deleteTexture(this.texture),this.framebuffer&&r.deleteFramebuffer(this.framebuffer),this.create()}}},{key:"bindAsRenderTarget",value:function(){var t=this.gl;t.bindFramebuffer(t.FRAMEBUFFER,this.framebuffer),t.viewport(0,0,this.textureWidth,this.textureHeight)}},{key:"bind",value:function(t){var r=this.gl;r.activeTexture(r.TEXTURE0+t),r.bindTexture(r.TEXTURE_2D,this.texture)}},{key:"getTexture",value:function(){return this.texture}},{key:"getTextureWidth",value:function(){return this.textureWidth}},{key:"getTextureHeight",value:function(){return this.textureHeight}},{key:"restore",value:function(){this.gl.getExtension("EXT_color_buffer_float"),this.create()}},{key:"kill",value:function(){var t=this.gl;this.texture&&(t.deleteTexture(this.texture),this.texture=null),this.framebuffer&&(t.deleteFramebuffer(this.framebuffer),this.framebuffer=null)}}])})(),bs=(function(e){function n(t,r){return ee(this,n),ge(this,n,[t,2,r])}return pe(n,e),te(n,[{key:"updateEdge",value:function(r,a,i,o,s,l){var u=arguments.length>6&&arguments[6]!==void 0?arguments[6]:0,d=arguments.length>7&&arguments[7]!==void 0?arguments[7]:0,c=arguments.length>8&&arguments[8]!==void 0?arguments[8]:0,g=this.indexMap.get(r);if(g===void 0)throw new Error('Edge "'.concat(r,'" not allocated in EdgeDataTexture'));var h=g*this.TEXELS_PER_ITEM*4;this.data[h+0]=a,this.data[h+1]=i,this.data[h+2]=o,this.data[h+3]=0,this.data[h+4]=s,this.data[h+5]=l,this.data[h+6]=u,this.data[h+7]=(d&15)<<4|c&15,this.markDirty(g)}}])})(Mr);var Bm=it(_n()),Nm=it(an());function _f(e,n){if(e==null)return{};var t={};for(var r in e)if({}.hasOwnProperty.call(e,r)){if(n.indexOf(r)!==-1)continue;t[r]=e[r]}return t}function ys(e,n){if(e==null)return{};var t,r,a=_f(e,n);if(Object.getOwnPropertySymbols){var i=Object.getOwnPropertySymbols(e);for(r=0;r<i.length;r++)t=i[r],n.indexOf(t)===-1&&{}.propertyIsEnumerable.call(e,t)&&(a[t]=e[t])}return a}var xf=["factor"],Tf=["factor"],_s=1.5,Xa={x:.5,y:.5,angle:0,ratio:1},xs=(function(e){function n(){var t;ee(this,n);for(var r=arguments.length,a=new Array(r),i=0;i<r;i++)a[i]=arguments[i];return t=ge(this,n,[].concat(a)),D(t,"minRatio",null),D(t,"maxRatio",null),D(t,"enabled",!0),D(t,"enabledZooming",!0),D(t,"enabledPanning",!0),D(t,"enabledRotation",!0),D(t,"constrainState",null),D(t,"state",O({},Xa)),D(t,"previousState",O({},Xa)),D(t,"nextFrame",null),D(t,"currentAnimationFrom",null),D(t,"currentAnimationTo",null),D(t,"animationCallback",null),t}return pe(n,e),te(n,[{key:"x",get:function(){return this.state.x}},{key:"y",get:function(){return this.state.y}},{key:"angle",get:function(){return this.state.angle}},{key:"ratio",get:function(){return this.state.ratio}},{key:"getState",value:function(){return O({},this.state)}},{key:"getPreviousState",value:function(){return O({},this.previousState)}},{key:"getBoundedRatio",value:function(r){var a=r;return typeof this.minRatio=="number"&&(a=Math.max(a,this.minRatio)),typeof this.maxRatio=="number"&&(a=Math.min(a,this.maxRatio)),a}},{key:"validateState",value:function(r){var a=this.getState();return this.enabledPanning&&typeof r.x=="number"&&(a.x=r.x),this.enabledPanning&&typeof r.y=="number"&&(a.y=r.y),this.enabledZooming&&typeof r.ratio=="number"&&(a.ratio=this.getBoundedRatio(r.ratio)),this.enabledRotation&&typeof r.angle=="number"&&(a.angle=r.angle),this.constrainState?this.constrainState(a):a}},{key:"isAnimating",value:function(){return this.nextFrame!==null}},{key:"setState",value:function(r){if(!this.enabled)return this;var a=this.validateState(r);return Ro(this.state,a)?this:(this.previousState=this.state,this.state=a,this.emit("updated",this.getState()),this)}},{key:"updateState",value:function(r){return this.setState(r(this.getState())),this}},{key:"animate",value:function(r,a){var i=this;return this.enabled?new Promise(function(o){return i.runAnimation(r,O(O({},wo),a),o)}):Promise.resolve()}},{key:"cancelAnimation",value:function(){return this.nextFrame!==null&&(cancelAnimationFrame(this.nextFrame),this.nextFrame=null),this.resolveAnimation(),this.emitAnimationEnd(!1),this}},{key:"zoomIn",value:function(){var r=arguments.length>0&&arguments[0]!==void 0?arguments[0]:{},a=r.factor,i=a===void 0?_s:a,o=ys(r,xf);return this.animate({ratio:this.ratio/i},o)}},{key:"zoomOut",value:function(){var r=arguments.length>0&&arguments[0]!==void 0?arguments[0]:{},a=r.factor,i=a===void 0?_s:a,o=ys(r,Tf);return this.animate({ratio:this.ratio*i},o)}},{key:"reset",value:function(r){return this.animate(Xa,r)}},{key:"runAnimation",value:function(r,a,i){var o=this,s=Tr(a.easing),l=Date.now(),u=this.getState(),d=this.validateState(r),c=function(){if(!o.enabled){o.cancelAnimation();return}var h=a.duration>0?(Date.now()-l)/a.duration:1;if(h>=1){o.nextFrame=null,o.setState(d),o.resolveAnimation(),o.emitAnimationEnd(!0);return}var f=s(h);o.setState({x:u.x+(d.x-u.x)*f,y:u.y+(d.y-u.y)*f,angle:u.angle+(d.angle-u.angle)*f,ratio:u.ratio+(d.ratio-u.ratio)*f}),o.nextFrame=requestAnimationFrame(c)};this.cancelAnimation(),this.currentAnimationFrom=u,this.currentAnimationTo=d,this.animationCallback=i,this.emit("animationStart",{from:u,to:d}),c()}},{key:"resolveAnimation",value:function(){var r=this.animationCallback;this.animationCallback=null,r&&r()}},{key:"emitAnimationEnd",value:function(r){var a=this.currentAnimationFrom,i=this.currentAnimationTo;!a||!i||(this.currentAnimationFrom=null,this.currentAnimationTo=null,this.emit("animationEnd",{from:a,to:i,completed:r}))}}],[{key:"from",value:function(r){var a=new n;return a.setState(r)}}])})(Ar);function Ge(e,n){var t=n.getBoundingClientRect();return{x:e.clientX-t.left,y:e.clientY-t.top}}function Qe(e,n){var t=O(O({},Ge(e,n)),{},{sigmaDefaultPrevented:!1,preventSigmaDefault:function(){t.sigmaDefaultPrevented=!0},original:e});return t}function zt(e){var n="x"in e?e:O(O({},e.touches[0]||e.previousTouches[0]),{},{original:e.original,sigmaDefaultPrevented:e.sigmaDefaultPrevented,preventSigmaDefault:function(){e.sigmaDefaultPrevented=!0,n.sigmaDefaultPrevented=!0}});return n}function Ef(e,n){var t=Qe(e,n);return t.delta=zs(e),t}var Sf=2;function Vr(e){for(var n=[],t=0,r=Math.min(e.length,Sf);t<r;t++)n.push(e[t]);return n}function zn(e,n,t){var r={touches:Vr(e.touches).map(function(a){return Ge(a,t)}),previousTouches:n.map(function(a){return Ge(a,t)}),sigmaDefaultPrevented:!1,preventSigmaDefault:function(){r.sigmaDefaultPrevented=!0},original:e};return r}function zs(e){if(typeof e.deltaY<"u")return e.deltaY*-3/360;if(typeof e.detail<"u")return e.detail/-9;throw new Error("Captor: could not extract delta from event.")}var Bs=(function(e){function n(t,r){var a;return ee(this,n),a=ge(this,n),a.container=t,a.renderer=r,a}return pe(n,e),te(n)})(Ar),Rf=["doubleClickTimeout","doubleClickZoomingDuration","doubleClickZoomingRatio","dragTimeout","draggedEventsTolerance","enableCameraMouseRotation","gestureTarget","inertiaDuration","inertiaRatio","zoomDuration","zoomingRatio"],Af=Rf.reduce(function(e,n){return O(O({},e),{},D({},n,kr[n]))},{});function Bn(e){return e instanceof PointerEvent&&e.pointerType==="touch"}var Df=(function(e){function n(t,r){var a;return ee(this,n),a=ge(this,n,[t,r]),D(a,"enabled",!0),D(a,"draggedEvents",0),D(a,"downStartTime",null),D(a,"lastMouseX",null),D(a,"lastMouseY",null),D(a,"isMouseDown",!1),D(a,"isMoving",!1),D(a,"isPanningStage",!1),D(a,"movingTimeout",null),D(a,"startCameraState",null),D(a,"clicks",0),D(a,"doubleClickTimeout",null),D(a,"isRightMouseDown",!1),D(a,"startRotationAngle",null),D(a,"startCameraAngle",null),D(a,"currentWheelDirection",0),D(a,"lastWheelAnimationId",0),D(a,"settings",Af),a.handleRightClick=a.handleRightClick.bind(a),a.handleDown=a.handleDown.bind(a),a.handleUp=a.handleUp.bind(a),a.handleMove=a.handleMove.bind(a),a.handleWheel=a.handleWheel.bind(a),a.handleLeave=a.handleLeave.bind(a),a.handleEnter=a.handleEnter.bind(a),t.addEventListener("contextmenu",a.handleRightClick,{capture:!1}),t.addEventListener("pointerdown",a.handleDown,{capture:!1}),t.addEventListener("wheel",a.handleWheel,{capture:!1,passive:!1}),t.addEventListener("pointerleave",a.handleLeave,{capture:!1}),t.addEventListener("pointerenter",a.handleEnter,{capture:!1}),document.addEventListener("pointermove",a.handleMove,{capture:!1}),document.addEventListener("pointerup",a.handleUp,{capture:!1}),a}return pe(n,e),te(n,[{key:"kill",value:function(){var r=this.container;r.removeEventListener("contextmenu",this.handleRightClick),r.removeEventListener("pointerdown",this.handleDown),r.removeEventListener("wheel",this.handleWheel),r.removeEventListener("pointerleave",this.handleLeave),r.removeEventListener("pointerenter",this.handleEnter),document.removeEventListener("pointermove",this.handleMove),document.removeEventListener("pointerup",this.handleUp)}},{key:"handleClick",value:function(r){var a=this;if(this.enabled){if(this.clicks++,this.clicks===2)return this.clicks=0,typeof this.doubleClickTimeout=="number"&&(clearTimeout(this.doubleClickTimeout),this.doubleClickTimeout=null),this.handleDoubleClick(r);setTimeout(function(){a.clicks=0,a.doubleClickTimeout=null},this.settings.doubleClickTimeout),this.draggedEvents<this.settings.draggedEventsTolerance&&this.emit("click",Qe(r,this.container))}}},{key:"handleRightClick",value:function(r){this.enabled&&(this.settings.enableCameraMouseRotation&&r.preventDefault(),this.emit("rightClick",Qe(r,this.container)))}},{key:"handleDoubleClick",value:function(r){if(this.enabled){r.preventDefault(),r.stopPropagation();var a=Qe(r,this.container);if(this.emit("doubleClick",a),!a.sigmaDefaultPrevented){var i=this.renderer.getCamera(),o=i.getBoundedRatio(i.getState().ratio/this.settings.doubleClickZoomingRatio);i.animate(this.renderer.getViewportZoomedState(Ge(r,this.container),o),{easing:"quadraticInOut",duration:this.settings.doubleClickZoomingDuration})}}}},{key:"handleDown",value:function(r){if(!(!this.enabled||Bn(r))){if(r.button===0){this.startCameraState=this.renderer.getCamera().getState();var a=Ge(r,this.container),i=a.x,o=a.y;this.lastMouseX=i,this.lastMouseY=o,this.draggedEvents=0,this.downStartTime=Date.now(),this.isMouseDown=!0}if(r.button===2&&this.settings.enableCameraMouseRotation){var s=Ge(r,this.container),l=s.x,u=s.y,d=this.container.offsetWidth/2,c=this.container.offsetHeight/2;this.startRotationAngle=Math.atan2(u-c,l-d),this.startCameraAngle=this.renderer.getCamera().getState().angle,this.isRightMouseDown=!0}this.emit("mousedown",Qe(r,this.container))}}},{key:"handleUp",value:function(r){var a=this;if(!(!this.enabled||Bn(r)||!this.isMouseDown&&!this.isRightMouseDown)){if(this.isRightMouseDown){this.isRightMouseDown=!1,this.startRotationAngle=null,this.startCameraAngle=null,this.emit("mouseup",Qe(r,this.container));return}var i=this.renderer.getCamera();this.isMouseDown=!1,this.isPanningStage&&(this.isPanningStage=!1,this.renderer._setPanning(!1)),typeof this.movingTimeout=="number"&&(clearTimeout(this.movingTimeout),this.movingTimeout=null);var o=Ge(r,this.container),s=o.x,l=o.y,u=i.getState(),d=i.getPreviousState();this.isMoving?i.animate({x:u.x+this.settings.inertiaRatio*(u.x-d.x),y:u.y+this.settings.inertiaRatio*(u.y-d.y)},{duration:this.settings.inertiaDuration,easing:"quadraticOut"}):(this.lastMouseX!==s||this.lastMouseY!==l)&&i.setState({x:u.x,y:u.y}),this.isMoving=!1,setTimeout(function(){var c=a.draggedEvents>0;a.draggedEvents=0;var g=a.renderer.getSetting("hideEdgesOnMove")||a.renderer.getSetting("hideLabelsOnMove");c&&g&&a.renderer.refresh()},0),this.emit("mouseup",Qe(r,this.container)),(r.target===this.container||r.composedPath()[0]===this.container)&&this.handleClick(r)}}},{key:"handleMove",value:function(r){var a=this;if(!(!this.enabled||Bn(r))){var i=Qe(r,this.container);if(this.emit("mousemovebody",i),(r.target===this.container||r.composedPath()[0]===this.container)&&this.emit("mousemove",i),this.isMouseDown&&this.draggedEvents++,!i.sigmaDefaultPrevented){if(this.isMouseDown){this.isMoving=!0,typeof this.movingTimeout=="number"&&clearTimeout(this.movingTimeout),this.movingTimeout=window.setTimeout(function(){a.movingTimeout=null,a.isMoving=!1},this.settings.dragTimeout);var o=this.renderer.getCamera(),s=Ge(r,this.container),l=s.x,u=s.y,d=this.renderer.viewportToFramedGraph({x:this.lastMouseX,y:this.lastMouseY}),c=this.renderer.viewportToFramedGraph({x:l,y:u}),g=d.x-c.x,h=d.y-c.y,f=o.getState(),y=f.x+g,v=f.y+h;this.isPanningStage||(this.isPanningStage=!0,this.renderer._setPanning(!0)),o.setState({x:y,y:v}),this.lastMouseX=l,this.lastMouseY=u,r.preventDefault(),r.stopPropagation()}if(this.isRightMouseDown){var p=Ge(r,this.container),m=p.x,b=p.y,x=this.container.offsetWidth/2,_=this.container.offsetHeight/2,T=Math.atan2(b-_,m-x),R=T-this.startRotationAngle,A=this.renderer.getCamera();A.setState({angle:this.startCameraAngle+R}),r.preventDefault(),r.stopPropagation()}}}}},{key:"handleLeave",value:function(r){Bn(r)||this.emit("mouseleave",Qe(r,this.container))}},{key:"handleEnter",value:function(r){Bn(r)||this.emit("mouseenter",Qe(r,this.container))}},{key:"handleWheel",value:function(r){var a=this,i=this.renderer.getCamera();if(!(!this.enabled||!i.enabledZooming)){var o=Ef(r,this.container);if(this.emit("wheel",o),!o.sigmaDefaultPrevented){var s=this.settings.gestureTarget;if(s!=="page"){if(s==="shared"&&!r.ctrlKey&&!r.metaKey){this.renderer._showGestureHint("wheel");return}var l=zs(r);if(l){r.preventDefault(),r.stopPropagation();var u=i.getState().ratio,d=l>0?1/this.settings.zoomingRatio:this.settings.zoomingRatio,c=i.getBoundedRatio(u*d),g=l>0?1:-1,h=Date.now();if(u!==c&&!(this.currentWheelDirection===g&&this.lastWheelTriggerTime&&h-this.lastWheelTriggerTime<this.settings.zoomDuration/5)){var f=++this.lastWheelAnimationId;i.animate(this.renderer.getViewportZoomedState(Ge(r,this.container),c),{easing:"quadraticOut",duration:this.settings.zoomDuration}).then(function(){a.lastWheelAnimationId===f&&(a.currentWheelDirection=0)}),this.currentWheelDirection=g,this.lastWheelTriggerTime=h}}}}}}},{key:"setSettings",value:function(r){this.settings=r}}])})(Bs),wf=["dragTimeout","gestureTarget","inertiaDuration","inertiaRatio","doubleClickTimeout","doubleClickZoomingRatio","doubleClickZoomingDuration","tapMoveTolerance"],kf=wf.reduce(function(e,n){return O(O({},e),{},D({},n,kr[n]))},{}),Cf=(function(e){function n(t,r){var a;return ee(this,n),a=ge(this,n,[t,r]),D(a,"enabled",!0),D(a,"isMoving",!1),D(a,"hasMoved",!1),D(a,"isPanningStage",!1),D(a,"isZoomingStage",!1),D(a,"touchMode",0),D(a,"startTouchesPositions",[]),D(a,"lastTouches",[]),D(a,"lastTap",null),D(a,"settings",kf),a.handleStart=a.handleStart.bind(a),a.handleLeave=a.handleLeave.bind(a),a.handleMove=a.handleMove.bind(a),t.addEventListener("touchstart",a.handleStart,{capture:!1}),t.addEventListener("touchcancel",a.handleLeave,{capture:!1}),document.addEventListener("touchend",a.handleLeave,{capture:!1,passive:!1}),document.addEventListener("touchmove",a.handleMove,{capture:!1,passive:!1}),a}return pe(n,e),te(n,[{key:"kill",value:function(){var r=this.container;r.removeEventListener("touchstart",this.handleStart),r.removeEventListener("touchcancel",this.handleLeave),document.removeEventListener("touchend",this.handleLeave),document.removeEventListener("touchmove",this.handleMove)}},{key:"getDimensions",value:function(){return{width:this.container.offsetWidth,height:this.container.offsetHeight}}},{key:"doesCapture",value:function(r){var a=this.settings.gestureTarget;return a==="graph"?!0:a==="page"?!1:r>=2}},{key:"syncStageFlags",value:function(){var r=this.touchMode===1&&this.hasMoved,a=this.touchMode===2;r!==this.isPanningStage&&(this.isPanningStage=r,this.renderer._setPanning(r)),a!==this.isZoomingStage&&(this.isZoomingStage=a,this.renderer._setZooming(a))}},{key:"handleStart",value:function(r){var a=this;if(this.enabled){var i=Vr(r.touches);if(this.touchMode=i.length,this.startCameraState=this.renderer.getCamera().getState(),this.startTouchesPositions=i.map(function(h){return Ge(h,a.container)}),this.touchMode===2){var o=re(this.startTouchesPositions,2),s=o[0],l=s.x,u=s.y,d=o[1],c=d.x,g=d.y;this.startTouchesAngle=Math.atan2(g-u,c-l),this.startTouchesDistance=Math.sqrt(Math.pow(c-l,2)+Math.pow(g-u,2))}this.syncStageFlags(),this.emit("touchdown",zn(r,this.lastTouches,this.container)),this.lastTouches=i,this.lastTouchesPositions=this.startTouchesPositions,r.cancelable&&(this.doesCapture(r.touches.length)||this.renderer._hasNodeDrag())&&r.preventDefault()}}},{key:"handleLeave",value:function(r){if(!(!this.enabled||!this.startTouchesPositions.length)){switch(r.cancelable&&this.doesCapture(this.touchMode)&&r.preventDefault(),this.movingTimeout&&(this.isMoving=!1,clearTimeout(this.movingTimeout)),this.touchMode){case 2:if(r.touches.length===1){this.handleStart(r);break}case 1:if(this.isMoving){var a=this.renderer.getCamera(),i=a.getState(),o=a.getPreviousState();a.animate({x:i.x+this.settings.inertiaRatio*(i.x-o.x),y:i.y+this.settings.inertiaRatio*(i.y-o.y)},{duration:this.settings.inertiaDuration,easing:"quadraticOut"})}this.hasMoved=!1,this.isMoving=!1,this.touchMode=0;break}if(this.syncStageFlags(),this.emit("touchup",zn(r,this.lastTouches,this.container)),!r.touches.length){var s=Ge(this.lastTouches[0],this.container),l=this.startTouchesPositions[0],u=Math.pow(s.x-l.x,2)+Math.pow(s.y-l.y,2);if(!r.touches.length&&u<Math.pow(this.settings.tapMoveTolerance,2))if(this.lastTap&&Date.now()-this.lastTap.time<this.settings.doubleClickTimeout){var d=zn(r,this.lastTouches,this.container);if(this.emit("doubletap",d),this.lastTap=null,!d.sigmaDefaultPrevented&&this.settings.gestureTarget!=="page"){var c=this.renderer.getCamera(),g=c.getBoundedRatio(c.getState().ratio/this.settings.doubleClickZoomingRatio);c.animate(this.renderer.getViewportZoomedState(s,g),{easing:"quadraticInOut",duration:this.settings.doubleClickZoomingDuration})}}else{var h=zn(r,this.lastTouches,this.container);this.emit("tap",h),this.lastTap={time:Date.now(),position:h.touches[0]||h.previousTouches[0]}}}this.lastTouches=Vr(r.touches),this.startTouchesPositions=[]}}},{key:"handleMove",value:function(r){var a=this;if(!(!this.enabled||!this.startTouchesPositions.length)){var i=this.doesCapture(r.touches.length);i&&r.preventDefault();var o=Vr(r.touches),s=o.map(function(w){return Ge(w,a.container)}),l=this.lastTouches;this.lastTouches=o,this.lastTouchesPositions=s;var u=zn(r,l,this.container);if(this.emit("touchmove",u),!u.sigmaDefaultPrevented&&(this.hasMoved||(this.hasMoved=s.some(function(w,$){var j=a.startTouchesPositions[$];return j&&(w.x!==j.x||w.y!==j.y)})),!!this.hasMoved)){if(!i){this.settings.gestureTarget==="shared"&&this.renderer._showGestureHint("touch");return}this.isMoving=!0,this.syncStageFlags(),this.movingTimeout&&clearTimeout(this.movingTimeout),this.movingTimeout=window.setTimeout(function(){a.isMoving=!1},this.settings.dragTimeout);var d=this.renderer.getCamera(),c=this.startCameraState,g=this.renderer.getSetting("stagePadding");switch(this.touchMode){case 1:{var h=this.renderer.viewportToFramedGraph((this.startTouchesPositions||[])[0]),f=h.x,y=h.y,v=this.renderer.viewportToFramedGraph(s[0]),p=v.x,m=v.y;d.setState({x:c.x+f-p,y:c.y+y-m});break}case 2:{var b={x:.5,y:.5,angle:0,ratio:1},x=s[0],_=x.x,T=x.y,R=s[1],A=R.x,S=R.y,k=Math.atan2(S-T,A-_)-this.startTouchesAngle,L=Math.hypot(S-T,A-_)/this.startTouchesDistance,C=d.getBoundedRatio(c.ratio/L);b.ratio=C,b.angle=c.angle+k;var z=this.getDimensions(),F=this.renderer.viewportToFramedGraph((this.startTouchesPositions||[])[0],{cameraState:c}),P=Math.min(z.width,z.height)-2*g,N=P/z.width,G=P/z.height,E=C/P,I=_-P/2/N,W=T-P/2/G,B=fa({x:I,y:W},b.angle);I=B.x,W=B.y,b.x=F.x-I*E,b.y=F.y+W*E,d.setState(b);break}}}}}},{key:"setSettings",value:function(r){this.settings=r}}])})(Bs),Ff=(function(){function e(n,t,r,a){ee(this,e),D(this,"pendingNode",null),D(this,"session",null),this.graph=n,this.viewportToGraph=t,this.setNodesState=r,this.emit=a}return te(e,[{key:"start",value:function(t,r,a,i,o){var s=a(t),l=!1;if(this.emit("nodeDragStart",{node:t,allDraggedNodes:s,event:r,preventSigmaDefault:function(){l=!0}}),l)return!1;var u=new Map,d=U(s),c;try{for(d.s();!(c=d.n()).done;){var g=c.value;u.set(g,{x:this.graph.getNodeAttribute(g,i),y:this.graph.getNodeAttribute(g,o)})}}catch(h){d.e(h)}finally{d.f()}return this.session={node:t,allNodes:s,startPosition:this.viewportToGraph(r),startNodePositions:u,xAttr:i,yAttr:o},this.setNodesState(s,{isDragged:!0}),!0}},{key:"applyMove",value:function(t,r){var a=this.session,i=a.allNodes,o=a.startNodePositions,s=a.startPosition,l=a.xAttr,u=a.yAttr,d=this.viewportToGraph(t),c={x:d.x-s.x,y:d.y-s.y},g=U(i),h;try{for(g.s();!(h=g.n()).done;){var f=h.value,y=o.get(f);if(!(!y||!this.graph.hasNode(f))){var v={x:y.x+c.x,y:y.y+c.y};r?this.graph.mergeNodeAttributes(f,r(v,f)):(this.graph.setNodeAttribute(f,l,v.x),this.graph.setNodeAttribute(f,u,v.y))}}}catch(p){g.e(p)}finally{g.f()}}},{key:"end",value:function(){if(this.pendingNode=null,!this.session)return null;var t=this.session,r=t.node,a=t.allNodes;return this.setNodesState(a,{isDragged:!1}),this.session=null,{node:r,allNodes:a}}},{key:"removeNode",value:function(t){t===this.pendingNode&&(this.pendingNode=null),this.session&&(this.session.node===t?(this.setNodesState(this.session.allNodes,{isDragged:!1}),this.session=null):this.session.allNodes.includes(t)&&(this.session.allNodes=this.session.allNodes.filter(function(r){return r!==t}),this.session.startNodePositions.delete(t)))}},{key:"clear",value:function(){this.pendingNode=null,this.session=null}}])})();function Ts(e,n){return e<n?"".concat(e,"\0").concat(n):"".concat(n,"\0").concat(e)}var Lf=(function(){function e(n,t){ee(this,e),D(this,"groups",new Map),D(this,"edgeToGroupKey",new Map),this.graph=n,this.onGroupChanged=t}return te(e,[{key:"getGroupKey",value:function(t){return Ts(this.graph.source(t),this.graph.target(t))}},{key:"sortGroup",value:function(t,r){var a=this,i=r.split("\0")[0];t.sort(function(o,s){var l=a.graph.source(o)===i||!a.graph.isDirected(o)?0:1,u=a.graph.source(s)===i||!a.graph.isDirected(s)?0:1;return l-u})}},{key:"register",value:function(t){var r=this.getGroupKey(t),a=this.groups.get(r);a||(a=[],this.groups.set(r,a)),a.includes(t)||a.push(t),this.edgeToGroupKey.set(t,r),a.length>1&&this.sortGroup(a,r),this.onGroupChanged(a,a.length)}},{key:"unregister",value:function(t){var r=this.edgeToGroupKey.get(t);if(r){this.edgeToGroupKey.delete(t);var a=this.groups.get(r);if(a){var i=a.indexOf(t);i!==-1&&a.splice(i,1),a.length===0?this.groups.delete(r):this.onGroupChanged(a,a.length)}}}},{key:"getGroup",value:function(t){var r,a=this.edgeToGroupKey.get(t);return a?(r=this.groups.get(a))!==null&&r!==void 0?r:[]:[]}},{key:"getSiblings",value:function(t){return this.getGroup(t).filter(function(r){return r!==t})}},{key:"rebuild",value:function(){var t=this;this.groups.clear(),this.edgeToGroupKey.clear(),this.graph.forEachEdge(function(l,u,d,c){var g=Ts(d,c),h=t.groups.get(g);h||(h=[],t.groups.set(g,h)),h.push(l),t.edgeToGroupKey.set(l,g)});var r=U(this.groups),a;try{for(r.s();!(a=r.n()).done;){var i=re(a.value,2),o=i[0],s=i[1];s.length>1&&this.sortGroup(s,o),this.onGroupChanged(s,s.length)}}catch(l){r.e(l)}finally{r.f()}}},{key:"clear",value:function(){this.groups.clear(),this.edgeToGroupKey.clear()}}])})();function Es(e,n){var t;if(e===!1||e==="extend"||e==="separate")return e;var r=e[n];return r!==void 0?r:(t=e.default)!==null&&t!==void 0?t:!1}function Ss(e){return e===!1?!1:e==="extend"||e==="separate"?!0:Object.values(e).some(function(n){return n==="extend"||n==="separate"})}function Rs(e){return e==="separate"?!0:e===!1||e==="extend"?!1:e.default==="separate"?!0:Object.values(e).includes("separate")}var ke={node:{parent:null,eventSuffix:"Node",payloadKey:"node",isEnabled:function(){return!0},writesPickingThisFrame:function(){return!0},setHover:function(n,t,r){return n.setNodeState(t,{isHovered:r})},getCursor:function(n,t){var r;return(r=n.nodeDataCache[t])===null||r===void 0?void 0:r.cursor},isHitValid:function(n,t){var r;return((r=n.nodeDataCache[t])===null||r===void 0?void 0:r.visibility)!=="hidden"}},edge:{parent:null,eventSuffix:"Edge",payloadKey:"edge",isEnabled:function(n){return!!n.settings.enableEdgeEvents},writesPickingThisFrame:function(n){return!!n.settings.enableEdgeEvents},setHover:function(n,t,r){return n.setEdgeState(t,{isHovered:r})},getCursor:function(n,t){var r;return(r=n.edgeDataCache[t])===null||r===void 0?void 0:r.cursor}},nodeLabel:{parent:"node",eventSuffix:"NodeLabel",payloadKey:"node",isEnabled:function(n){return Rs(n.settings.nodeLabelEvents)},writesPickingThisFrame:function(n){return Ss(n.settings.nodeLabelEvents)},setHover:function(n,t,r){return n.setNodeState(t,{isLabelHovered:r})},getCursor:function(n,t){var r;return(r=n.nodeDataCache[t])===null||r===void 0?void 0:r.labelCursor},resolveForVerb:function(n,t,r){var a=Es(r.settings.nodeLabelEvents,t);return a===!1?null:a==="extend"?{kind:"node",key:n.key}:n}},edgeLabel:{parent:"edge",eventSuffix:"EdgeLabel",payloadKey:"edge",isEnabled:function(n){return Rs(n.settings.edgeLabelEvents)},writesPickingThisFrame:function(n){return Ss(n.settings.edgeLabelEvents)},setHover:function(n,t,r){return n.setEdgeState(t,{isLabelHovered:r})},getCursor:function(n,t){var r;return(r=n.edgeDataCache[t])===null||r===void 0?void 0:r.labelCursor},resolveForVerb:function(n,t,r){var a=Es(r.settings.edgeLabelEvents,t);return a===!1?null:a==="extend"?{kind:"edge",key:n.key}:n}}},Mt=["node","edge","nodeLabel","edgeLabel"];function Pf(){return{lookup:[null],idsByKind:{node:new Map,edge:new Map,nodeLabel:new Map,edgeLabel:new Map},nextId:1}}function Nt(e,n,t){var r;return(r=e.idsByKind[n].get(t))!==null&&r!==void 0?r:0}function Ya(e,n){var t=U(Mt),r;try{for(t.s();!(r=t.n()).done;){var a=r.value;if(a===n||ke[a].parent===n){var i=U(e.idsByKind[a].values()),o;try{for(i.s();!(o=i.n()).done;){var s=o.value;e.lookup[s]=null}}catch(g){i.e(g)}finally{i.f()}e.idsByKind[a]=new Map}}}catch(g){t.e(g)}finally{t.f()}var l=1,u=U(Mt),d;try{for(u.s();!(d=u.n()).done;){var c=d.value;if(c===n)break;l+=e.idsByKind[c].size}}catch(g){u.e(g)}finally{u.f()}e.nextId=l}function As(e,n,t){var r=e.nextId++;return e.idsByKind[n].set(t,r),e.lookup[r]={kind:n,key:t},r}function If(e,n){var t=U(Mt),r;try{for(t.s();!(r=t.n()).done;){var a=r.value;if(ke[a].parent!==null){var i=U(e.idsByKind[a].values()),o;try{for(i.s();!(o=i.n()).done;){var s=o.value;e.lookup[s]=null}}catch(_){i.e(_)}finally{i.f()}e.idsByKind[a]=new Map}}}catch(_){t.e(_)}finally{t.f()}e.nextId=1;var l=U(Mt),u;try{for(l.s();!(u=l.n()).done;){var d=u.value;if(ke[d].parent===null){var c=U(e.idsByKind[d].values()),g;try{for(c.s();!(g=c.n()).done;){var h=g.value;h>=e.nextId&&(e.nextId=h+1)}}catch(_){c.e(_)}finally{c.f()}}}}catch(_){l.e(_)}finally{l.f()}var f=U(Mt),y;try{for(f.s();!(y=f.n()).done;){var v=y.value,p=ke[v];if(!(p.parent===null||!p.isEnabled(n))){var m=U(e.idsByKind[p.parent].keys()),b;try{for(m.s();!(b=m.n()).done;){var x=b.value;e.idsByKind[v].set(x,e.nextId),e.lookup[e.nextId]={kind:v,key:x},e.nextId++}}catch(_){m.e(_)}finally{m.f()}}}}catch(_){f.e(_)}finally{f.f()}}function Gf(e,n){var t=U(Mt),r;try{for(t.s();!(r=t.n()).done;){var a=r.value,i=e[a],o=n[a];if(i!==o){if(i.size!==o.size)return!1;var s=U(i),l;try{for(s.s();!(l=s.n()).done;){var u=re(l.value,2),d=u[0],c=u[1];if(o.get(d)!==c)return!1}}catch(g){s.e(g)}finally{s.f()}}}}catch(g){t.e(g)}finally{t.f()}return!0}function Ds(e,n,t){ke[n.kind].setHover(e,n.key,t)}function zf(e,n){return ke[n.kind].getCursor(e,n.key)}function Nn(e,n){return"".concat(n).concat(ke[e].eventSuffix)}function Mn(e,n){return O(O({},n),{},D({},ke[e.kind].payloadKey,e.key))}function Bf(e,n,t){var r=ke[n.kind].resolveForVerb;return r?r(n,t,e):n}function Nf(e,n){var t=ke[n.kind].isHitValid;return t?t(e,n.key):!0}function Ja(e){return Mt.filter(function(n){return n===e||ke[n].parent===e})}function Ns(e,n,t){return n&&(n=Bf(e,n,t)),n&&!Nf(e,n)&&(n=null),n}function Ka(e,n,t){var r=t==="wheel"?e.stateManager.hovered:e.getHitAtPosition(n);return Ns(e,r,t)}function Mf(e,n){return!e||!n?e===n:e.kind===n.kind&&e.key===n.key}function Ms(e,n,t){var r={event:t,preventSigmaDefault:function(){return t.preventSigmaDefault()}},a=e.dragManager.session,i=a?{kind:"node",key:a.node}:Ns(e,n,"enter"),o=e.stateManager,s=o.hovered;Mf(s,i)||(s&&(Ds(e,s,!1),e.emit(Nn(s.kind,"leave"),Mn(s,r))),o.setHovered(i),i&&(Ds(e,i,!0),e.emit(Nn(i.kind,"enter"),Mn(i,r))),e.updateContainerCursor())}function Wf(e,n,t,r){r.handleResize=function(){return e.scheduleRefresh()},window.addEventListener("resize",r.handleResize),r.handleMove=function(i){e.hoverResolver.pointerMoved(zt(i))},r.handleMoveBody=function(i){var o=zt(i),s=e.dragManager;if(s.pendingNode&&!s.session){var l=e.nodeStyleAnalysis,u=l.xAttribute,d=l.yAttribute,c=e.settings;s.start(s.pendingNode,o,c.getDraggedNodes,u||"x",d||"y"),s.pendingNode=null}s.session&&(s.applyMove(o,e.settings.dragPositionToAttributes),e.emit("nodeDrag",{node:s.session.node,allDraggedNodes:s.session.allNodes,event:o}),o.preventSigmaDefault()),e.emit("moveBody",{event:o,preventSigmaDefault:function(){return o.preventSigmaDefault()}})},r.handleLeave=function(i){var o=zt(i),s={event:o,preventSigmaDefault:function(){return o.preventSigmaDefault()}};e.hoverResolver.pointerLeft(),Ms(e,null,o),e.emit("leaveStage",s)},r.handleEnter=function(i){var o=zt(i);e.emit("enterStage",{event:o,preventSigmaDefault:function(){return o.preventSigmaDefault()}})};var a=function(o){return function(s){var l=zt(s),u={event:l,preventSigmaDefault:function(){return l.preventSigmaDefault()}},d=Ka(e,l,o);if(d){e.emit(Nn(d.kind,o),Mn(d,u));return}e.emit("".concat(o,"Stage"),u)}};r.handleClick=a("click"),r.handleRightClick=a("rightClick"),r.handleDoubleClick=a("doubleClick"),r.handleWheel=a("wheel"),r.handleDown=function(i){var o=zt(i),s={event:o,preventSigmaDefault:function(){return o.preventSigmaDefault()}},l=Ka(e,o,"down");if(l){l.kind==="node"&&e.settings.enableNodeDrag&&(e.dragManager.pendingNode=l.key),e.emit(Nn(l.kind,"down"),Mn(l,s));return}e.emit("downStage",s)},r.handleUp=function(i){var o=zt(i),s={event:o,preventSigmaDefault:function(){return o.preventSigmaDefault()}},l=e.dragManager.end();l&&e.emit("nodeDragEnd",O({node:l.node,allDraggedNodes:l.allNodes},s));var u=Ka(e,o,"up");if(u){e.emit(Nn(u.kind,"up"),Mn(u,s));return}e.emit("upStage",s)},n.on("mousemove",r.handleMove),n.on("mousemovebody",r.handleMoveBody),n.on("click",r.handleClick),n.on("rightClick",r.handleRightClick),n.on("doubleClick",r.handleDoubleClick),n.on("wheel",r.handleWheel),n.on("mousedown",r.handleDown),n.on("mouseup",r.handleUp),n.on("mouseleave",r.handleLeave),n.on("mouseenter",r.handleEnter),t.on("touchdown",r.handleDown),t.on("touchdown",r.handleMove),t.on("touchup",r.handleUp),t.on("touchmove",r.handleMove),t.on("tap",r.handleClick),t.on("doubletap",r.handleDoubleClick),t.on("touchmove",r.handleMoveBody)}function Uf(e,n){var t=e.graph,r=new Set(["x","y","zIndex","type"]);n.eachNodeAttributesUpdatedGraphUpdate=function(a){var i,o=(i=a.hints)===null||i===void 0?void 0:i.attributes,s=!o||o.some(function(l){return r.has(l)});e.refresh({partialGraph:{nodes:t.nodes()},skipIndexation:!s,schedule:!0})},n.eachEdgeAttributesUpdatedGraphUpdate=function(a){var i,o=(i=a.hints)===null||i===void 0?void 0:i.attributes,s=o&&["zIndex","type"].some(function(l){return o?.includes(l)});e.refresh({partialGraph:{edges:t.edges()},skipIndexation:!s,schedule:!0})},n.addNodeGraphUpdate=function(a){e.addNode(a.key),e.refresh({partialGraph:{nodes:[a.key]},skipIndexation:!1,schedule:!0})},n.updateNodeGraphUpdate=function(a){e.refresh({partialGraph:{nodes:[a.key]},skipIndexation:!1,schedule:!0})},n.dropNodeGraphUpdate=function(a){e.removeNode(a.key),e.refresh({schedule:!0})},n.addEdgeGraphUpdate=function(a){var i=a.key;e.edgeGroups.register(i),e.addEdge(i);var o=e.edgeGroups.getSiblings(i),s=U(o),l;try{for(s.s();!(l=s.n()).done;){var u=l.value;e.addEdge(u)}}catch(d){s.e(d)}finally{s.f()}e.refresh({partialGraph:{edges:[i].concat(K(o))},schedule:!0})},n.updateEdgeGraphUpdate=function(a){e.refresh({partialGraph:{edges:[a.key]},skipIndexation:!1,schedule:!0})},n.dropEdgeGraphUpdate=function(a){var i=a.key,o=e.edgeGroups.getSiblings(i);e.edgeGroups.unregister(i),e.removeEdge(i);var s=U(o),l;try{for(s.s();!(l=s.n()).done;){var u=l.value;e.addEdge(u)}}catch(d){s.e(d)}finally{s.f()}e.refresh({schedule:!0})},n.clearEdgesGraphUpdate=function(){e.clearEdgeState(),e.clearEdgeIndices(),e.refresh({schedule:!0})},n.clearGraphUpdate=function(){e.clearEdgeState(),e.clearNodeState(),e.clearEdgeIndices(),e.clearNodeIndices(),e.refresh({schedule:!0})},t.on("nodeAdded",n.addNodeGraphUpdate),t.on("nodeDropped",n.dropNodeGraphUpdate),t.on("nodeAttributesUpdated",n.updateNodeGraphUpdate),t.on("eachNodeAttributesUpdated",n.eachNodeAttributesUpdatedGraphUpdate),t.on("edgeAdded",n.addEdgeGraphUpdate),t.on("edgeDropped",n.dropEdgeGraphUpdate),t.on("edgeAttributesUpdated",n.updateEdgeGraphUpdate),t.on("eachEdgeAttributesUpdated",n.eachEdgeAttributesUpdatedGraphUpdate),t.on("edgesCleared",n.clearEdgesGraphUpdate),t.on("cleared",n.clearGraphUpdate)}function Of(e,n){e.removeListener("nodeAdded",n.addNodeGraphUpdate),e.removeListener("nodeDropped",n.dropNodeGraphUpdate),e.removeListener("nodeAttributesUpdated",n.updateNodeGraphUpdate),e.removeListener("eachNodeAttributesUpdated",n.eachNodeAttributesUpdatedGraphUpdate),e.removeListener("edgeAdded",n.addEdgeGraphUpdate),e.removeListener("edgeDropped",n.dropEdgeGraphUpdate),e.removeListener("edgeAttributesUpdated",n.updateEdgeGraphUpdate),e.removeListener("eachEdgeAttributesUpdated",n.eachEdgeAttributesUpdatedGraphUpdate),e.removeListener("edgesCleared",n.clearEdgesGraphUpdate),e.removeListener("cleared",n.clearGraphUpdate)}var Hf={x:.5,y:.5,angle:0,ratio:1},jf=8,$f=.001;function Vf(e){var n=e.coords,t=e.nodeData,r=e.dimensions,a=e.stagePadding,i=e.zoomToSizeRatioFunction,o=e.itemSizesReference,s=e.fitLabels,l=e.nodeLabelBox,u=Object.keys(n);if(!u.length)return e.extent;for(var d=i(1)||1,c=o==="positions",g=r.width,h=r.height,f=e.extent,y=0;y<jf;y++){for(var v=wr(f),p={width:f.x[1]-f.x[0]||1,height:f.y[1]-f.y[0]||1},m=gt(Hf,r,p,a),b=ws(m,v({x:0,y:0}),g,h),x=ws(m,v({x:1,y:0}),g,h),_=Math.hypot(x.x-b.x,x.y-b.y)||1,T=1/0,R=-1/0,A=1/0,S=-1/0,k=0,L=u.length;k<L;k++){var C=u[k],z=t[C],F=z.size/d*(c?_:1),P=-F,N=F,G=-F,E=F;if(s){var I=l(z,F);I&&(I.minX<P&&(P=I.minX),I.maxX>N&&(N=I.maxX),I.minY<G&&(G=I.minY),I.maxY>E&&(E=I.maxY))}var W=n[C],B=W.x,w=W.y;T=Math.min(T,B+P/_),R=Math.max(R,B+N/_),A=Math.min(A,w-E/_),S=Math.max(S,w-G/_)}var $={x:[T,R],y:[A,S]},j=Math.max($.x[1]-$.x[0],$.y[1]-$.y[0])||1,J=Math.max(Math.abs($.x[0]-f.x[0]),Math.abs($.x[1]-f.x[1]),Math.abs($.y[0]-f.y[0]),Math.abs($.y[1]-f.y[1]));if(f=$,J/j<$f)break}return f}function ws(e,n,t,r){var a=Qt(e,n);return{x:(1+a.x)*t/2,y:(1-a.y)*r/2}}var qf=typeof navigator<"u"&&/Mac|iP(hone|ad|od)/.test(navigator.platform),Xf=1500,Yf=`.sigma-gesture-hint {
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
}`;function ks(e){var n,t,r=on("style");r.textContent=e,document.head.appendChild(r);var a=((n=(t=r.sheet)===null||t===void 0?void 0:t.cssRules.length)!==null&&n!==void 0?n:0)>0;return r.remove(),a}var Bt=null;function Jf(){return Bt===null&&(Bt=Yf,ks("@scope {}")&&(Bt=`@scope {
`.concat(Bt,`
}`)),ks("@layer sigma-gesture-hint {}")&&(Bt=`@layer sigma-gesture-hint {
`.concat(Bt,`
}`))),Bt}var Kf=(function(){function e(n){ee(this,e),D(this,"hideTimeout",null),this.styleElement=on("style"),this.styleElement.textContent=Jf(),n.appendChild(this.styleElement),this.element=on("div",{opacity:"0"},{class:"sigma-gesture-hint"}),n.appendChild(this.element)}return te(e,[{key:"show",value:function(t,r){var a=this,i=t==="touch"?r.sharedGestureTouchMessage:qf?r.sharedGestureAppleWheelMessage:r.sharedGestureWheelMessage;i&&(this.element.textContent=i,this.element.style.opacity="1",typeof this.hideTimeout=="number"&&clearTimeout(this.hideTimeout),this.hideTimeout=window.setTimeout(function(){a.hideTimeout=null,a.element.style.opacity="0"},Xf))}},{key:"kill",value:function(){typeof this.hideTimeout=="number"&&clearTimeout(this.hideTimeout),this.styleElement.remove(),this.element.remove()}}])})(),Zf=(function(){function e(n){ee(this,e),D(this,"readBuffer",null),D(this,"fence",null),D(this,"result",new Uint8Array(4)),D(this,"generation",0),D(this,"readGeneration",0),D(this,"lastEvent",null),D(this,"active",!1),D(this,"dirty",!1),D(this,"rafId",null),D(this,"killed",!1),this.options=n}return te(e,[{key:"pointerMoved",value:function(t){this.lastEvent=t,this.active=!0,!this.options.isRenderPending()&&this.request()}},{key:"pointerLeft",value:function(){this.active=!1,this.dirty=!1}},{key:"frameRendered",value:function(){this.request()}},{key:"invalidate",value:function(){this.generation++}},{key:"request",value:function(){if(!(this.killed||!this.active||!this.lastEvent)){if(this.fence){this.dirty=!0;return}this.startRead()}}},{key:"startRead",value:function(){var t=this.options,r=t.gl,a=t.getFrameBuffer,i=t.getPixelRatio,o=t.getDownSizingRatio;this.readGeneration=this.generation;var s=this.lastEvent,l=ga(r,s.x,s.y,i(),o()),u=re(l,2),d=u[0],c=u[1];this.readBuffer||(this.readBuffer=r.createBuffer()),r.bindFramebuffer(r.FRAMEBUFFER,a()),r.bindBuffer(r.PIXEL_PACK_BUFFER,this.readBuffer),r.bufferData(r.PIXEL_PACK_BUFFER,4,r.STREAM_READ),r.readPixels(d,c,1,1,r.RGBA,r.UNSIGNED_BYTE,0),r.bindBuffer(r.PIXEL_PACK_BUFFER,null),r.bindFramebuffer(r.FRAMEBUFFER,null),this.fence=r.fenceSync(r.SYNC_GPU_COMMANDS_COMPLETE,0),r.flush(),this.schedulePoll(s)}},{key:"schedulePoll",value:function(t){var r=this;this.rafId=requestAnimationFrame(function(){r.rafId=null,r.poll(t)})}},{key:"poll",value:function(t){var r=this.options,a=r.gl,i=r.onIndex;if(!(this.killed||!this.fence)){var o=a.clientWaitSync(this.fence,0,0);if(o===a.TIMEOUT_EXPIRED){this.schedulePoll(t);return}if(a.deleteSync(this.fence),this.fence=null,o!==a.WAIT_FAILED&&this.readGeneration===this.generation){a.bindBuffer(a.PIXEL_PACK_BUFFER,this.readBuffer),a.getBufferSubData(a.PIXEL_PACK_BUFFER,0,this.result),a.bindBuffer(a.PIXEL_PACK_BUFFER,null);var s=re(this.result,4),l=s[0],u=s[1],d=s[2],c=s[3];this.active&&i(pa(l,u,d,c),t)}this.dirty&&(this.dirty=!1,this.active&&this.lastEvent&&!this.options.isRenderPending()&&this.startRead())}}},{key:"reset",value:function(){this.rafId!==null&&cancelAnimationFrame(this.rafId),this.rafId=null,this.fence=null,this.readBuffer=null,this.dirty=!1}},{key:"kill",value:function(){this.killed=!0,this.rafId!==null&&cancelAnimationFrame(this.rafId),this.rafId=null;var t=this.options.gl;this.fence&&t.deleteSync(this.fence),this.fence=null,this.readBuffer&&t.deleteBuffer(this.readBuffer),this.readBuffer=null}}])})(),Cs=(function(){function e(n,t){ee(this,e),this.key=n,this.size=t}return te(e,null,[{key:"compare",value:function(t,r){return t.size>r.size?-1:t.size<r.size||t.key>r.key?1:-1}}])})(),$r=(function(){function e(){ee(this,e),D(this,"width",0),D(this,"height",0),D(this,"cellSize",0),D(this,"columns",0),D(this,"rows",0),D(this,"cells",{})}return te(e,[{key:"resizeAndClear",value:function(t,r){this.width=t.width,this.height=t.height,this.cellSize=r,this.columns=Math.ceil(t.width/r),this.rows=Math.ceil(t.height/r),this.cells={}}},{key:"getIndex",value:function(t){var r=Math.floor(t.x/this.cellSize),a=Math.floor(t.y/this.cellSize);return a*this.columns+r}},{key:"add",value:function(t,r,a){var i=new Cs(t,r),o=this.getIndex(a),s=this.cells[o];s||(s=[],this.cells[o]=s),s.push(i)}},{key:"organize",value:function(){for(var t in this.cells){var r=this.cells[t];r.sort(Cs.compare)}}},{key:"getLabelsToDisplay",value:function(t,r,a){var i=this.cellSize*this.cellSize,o=i/t/t,s=o*r/i,l=Math.ceil(s),u=[];if(a)for(var d=Math.max(0,Math.floor(a.x1/this.cellSize)),c=Math.min(this.columns-1,Math.floor(a.x2/this.cellSize)),g=Math.max(0,Math.floor(a.y1/this.cellSize)),h=Math.min(this.rows-1,Math.floor(a.y2/this.cellSize)),f=g;f<=h;f++)for(var y=d;y<=c;y++){var v=f*this.columns+y,p=this.cells[v];if(p)for(var m=0;m<Math.min(l,p.length);m++)u.push(p[m].key)}else for(var b in this.cells)for(var x=this.cells[b],_=0;_<Math.min(l,x.length);_++)u.push(x[_].key);return u}}])})();function Qf(e){var n=e.graph,t=e.hoveredNode,r=e.highlightedNodes,a=e.displayedNodeLabels,i=new Set,o=new Set(r);t&&o.add(t);var s=U(o),l;try{for(s.s();!(l=s.n()).done;){var u=l.value;n.hasNode(u)&&n.forEachEdge(u,function(h){i.add(h)})}}catch(h){s.e(h)}finally{s.f()}var d=U(a),c;try{var g=function(){var f=c.value;if(o.has(f))return 1;n.forEachEdge(f,function(y,v,p,m){var b=p===f?m:p;a.has(b)&&i.add(y)})};for(d.s();!(c=d.n()).done;)g()}catch(h){d.e(h)}finally{d.f()}return Array.from(i)}var gn=150,pn=50,eg={both:0,node:1,label:2},tg={over:0,above:1,below:2,auto:3},Fs=12,ng=3,rg=(function(){function e(n){ee(this,e),D(this,"labelGrid",new $r),D(this,"edgeAnchorGrid",new $r),D(this,"displayedNodeLabels",new Set),D(this,"displayedEdgeLabels",new Set),D(this,"edgeLabelCandidates",[]),D(this,"renderedNodeLabels",new Set),D(this,"labelSizeCache",new Map),D(this,"framePassPoints",new Float32Array(0)),D(this,"labelsDirty",!0),this.internals=n}return te(e,[{key:"resetFrame",value:function(){this.displayedNodeLabels=new Set,this.renderedNodeLabels=new Set,this.labelSizeCache.clear()}},{key:"clearEdgeLabels",value:function(){this.displayedEdgeLabels=new Set}},{key:"resetLabelGrid",value:function(){this.labelGrid=new $r,this.edgeAnchorGrid=new $r}},{key:"processWebGLLabels",value:function(t){for(var r,a=this.internals,i=a.labelProgram,o=a.primitives,s=a.nodeDataCache,l=(o==null||(r=o.nodes)===null||r===void 0||(r=r.label)===null||r===void 0||(r=r.font)===null||r===void 0?void 0:r.family)||"sans-serif",u=new Map,d=0,c=t.length;d<c;d++){var g=t[d],h=s[g];if(!(h.visibility==="hidden"||!h.label)){var f=h.labelFont||l,y=u.get(f);y?y.push(h.label):u.set(f,[h.label])}}var v=U(u),p;try{for(v.s();!(p=v.n()).done;){var m=re(p.value,2),b=m[0],x=m[1],_=Fr(b),T=_.family,R=_.weight,A=_.style,S=i.registerFont(T,R,A);i.ensureGlyphsReady(x,S)}}catch(k){v.e(k)}finally{v.f()}}},{key:"measureNodeLabel",value:function(t){var r,a;if(!t.label)return{width:0,height:0,textHeight:0};var i=this.internals,o=i.labelProgram,s=i.primitives,l=(r=t.labelSize)!==null&&r!==void 0?r:14,u=t.labelFont||(s==null||(a=s.nodes)===null||a===void 0||(a=a.label)===null||a===void 0||(a=a.font)===null||a===void 0?void 0:a.family)||"sans-serif",d="".concat(t.label,"|").concat(l,"|").concat(u);if(this.labelSizeCache.has(d))return this.labelSizeCache.get(d);var c=Fr(u),g=c.family,h=c.weight,f=c.style,y=o.registerFont(g,h,f),v=o.measureLabel(t.label,l,y);return this.labelSizeCache.set(d,v),v}},{key:"nodeLabelBox",value:function(t,r){var a,i,o,s,l;if(t.visibility==="hidden"||t.labelVisibility==="hidden")return null;var u=this.measureNodeLabel(t),d=u.width,c=u.height,g=u.textHeight;if(!d)return null;var h=(a=(i=this.internals.primitives)===null||i===void 0||(i=i.nodes)===null||i===void 0||(i=i.label)===null||i===void 0?void 0:i.margin)!==null&&a!==void 0?a:un,f=r+h,y=t.labelBackgroundColor?(o=t.labelBackgroundPadding)!==null&&o!==void 0?o:Ba:0,v=d/2,p=c/2,m=v+y,b=p+y,x=g/2,_=0,T=0;switch((s=t.labelPosition)!==null&&s!==void 0?s:"right"){case"left":_=-(f+v);break;case"above":T=-(f+x);break;case"below":T=f+x;break;case"over":break;default:_=f+v}var R=(l=t.labelAngle)!==null&&l!==void 0?l:0,A=fa({x:_,y:T},R),S=A.x,k=A.y,L=Math.abs(Math.cos(R)),C=Math.abs(Math.sin(R)),z=L*m+C*b,F=C*m+L*b;return{minX:S-z,maxX:S+z,minY:k-F,maxY:k+F}}},{key:"computeDisplayedNodeLabels",value:function(){var t=this.internals.getCameraState(),r=this.internals.getDimensions(),a=r.width,i=r.height,o=this.internals.viewportToFramedGraph({x:-gn,y:-pn}),s=this.internals.viewportToFramedGraph({x:a+gn,y:-pn}),l=this.internals.viewportToFramedGraph({x:-gn,y:i+pn}),u=this.internals.viewportToFramedGraph({x:a+gn,y:i+pn}),d=Math.min(o.x,s.x,l.x,u.x),c=Math.max(o.x,s.x,l.x,u.x),g=Math.min(o.y,s.y,l.y,u.y),h=Math.max(o.y,s.y,l.y,u.y),f=gt({x:.5,y:.5,ratio:1,angle:0},{width:a,height:i},this.internals.getGraphDimensions(),this.internals.getStagePadding()),y=function(I){var W=Qt(f,I);return{x:(1+W.x)*a/2,y:(1-W.y)*i/2}},v=y({x:d,y:g}),p=y({x:c,y:g}),m=y({x:d,y:h}),b=y({x:c,y:h}),x={x1:Math.min(v.x,p.x,m.x,b.x),y1:Math.min(v.y,p.y,m.y,b.y),x2:Math.max(v.x,p.x,m.x,b.x),y2:Math.max(v.y,p.y,m.y,b.y)},_=this.internals,T=_.settings,R=_.nodeDataCache,A=_.nodesWithForcedLabels,S=this.labelGrid.getLabelsToDisplay(t.ratio,T.labelDensity,x);Da(S,A);for(var k=0,L=S.length;k<L;k++){var C=S[k],z=R[C];if(!this.displayedNodeLabels.has(C)&&!(z.visibility==="hidden"||z.labelVisibility==="hidden")&&z.label&&!(z.x<d||z.x>c||z.y<g||z.y>h)){var F=this.internals.framedGraphToViewport(z),P=F.x,N=F.y,G=this.internals.scaleSize(z.size);!vt(z)&&G<T.labelRenderedSizeThreshold||P<-gn-G||P>a+gn+G||N<-pn-G||N>i+pn+G||this.displayedNodeLabels.add(C)}}}},{key:"buildFramePassPoints",value:function(){var t=this.internals,r=t.nodeDataCache,a=t.nodeDataTexture;if(!a)return{data:this.framePassPoints,count:0};var i=this.displayedNodeLabels.size*3;this.framePassPoints.length<i&&(this.framePassPoints=new Float32Array(i));var o=this.framePassPoints,s=0,l=U(this.displayedNodeLabels),u;try{for(l.s();!(u=l.n()).done;){var d,c,g=u.value,h=r[g];if(h){var f=a.getIndex(g);f<0||(o[s*3]=f,o[s*3+1]=(d=ln[h.labelPosition||ut.labelPosition])!==null&&d!==void 0?d:0,o[s*3+2]=(c=h.labelAngle)!==null&&c!==void 0?c:0,s++)}}}catch(y){l.e(y)}finally{l.f()}return{data:o,count:s}}},{key:"renderWebGLLabels",value:function(t,r){var a=this.internals,i=a.nodeDataCache,o=a.labelProgram,s=a.primitives,l=a.nodeDataTexture,u=[],d=U(this.displayedNodeLabels),c;try{for(d.s();!(c=d.n()).done;){var g=c.value;if(!this.renderedNodeLabels.has(g)){var h=i[g];r&&h.labelDepth!==r||(this.renderedNodeLabels.add(g),u.push(g))}}}catch(le){d.e(le)}finally{d.f()}if(u.length!==0){if(this.labelsDirty){for(var f,y,v,p=0,m=0,b=u.length;m<b;m++){var x=i[u[m]];p+=x.label.length}o.reallocate(p);for(var _=14,T=(f=s==null||(y=s.nodes)===null||y===void 0||(y=y.label)===null||y===void 0?void 0:y.margin)!==null&&f!==void 0?f:un,R=ut.labelPosition,A=(s==null||(v=s.nodes)===null||v===void 0||(v=v.label)===null||v===void 0||(v=v.font)===null||v===void 0?void 0:v.family)||"sans-serif",S=new Map,k=0,L=0,C=u.length;L<C;L++){var z,F,P,N,G=u[L],E=i[G],I=E.labelFont||A,W=S.get(I);if(W===void 0){var B=Fr(I),w=B.family,$=B.weight,j=B.style;W=o.registerFont(w,$,j),S.set(I,W)}var J={text:E.label,x:E.x,y:E.y,size:(z=E.labelSize)!==null&&z!==void 0?z:_,color:E.labelColor,nodeSize:E.size,margin:T,position:(F=E.labelPosition)!==null&&F!==void 0?F:R,hidden:!1,forceLabel:vt(E),type:"default",zIndex:(P=E.zIndex)!==null&&P!==void 0?P:0,parentType:"node",parentKey:G,fontKey:W,labelAngle:(N=E.labelAngle)!==null&&N!==void 0?N:0,nodeIndex:l.getIndex(G)},Se=o.processLabel(G,k,J);k+=Se}o.invalidateBuffers()}o.render(t)}}},{key:"renderBackdrops",value:function(t,r){var a=this.internals,i=a.backdropProgram,o=a.nodeDataCache,s=a.nodesWithBackdrop,l=a.attachmentManager,u=a.pixelRatio,d=a.nodeDataTexture,c=[],g=U(s),h;try{for(g.s();!(h=g.n()).done;){var f,y=h.value,v=o[y];if(!(!v||v.visibility==="hidden")&&!(r&&v.depth!==r)){var p=(f=d?.getIndex(y))!==null&&f!==void 0?f:-1;p<0||c.push({key:y,nodeIndex:p})}}}catch(X){g.e(X)}finally{g.f()}if(c.length!==0){i.reallocate(c.length);for(var m=0;m<c.length;m++){var b,x,_,T,R,A,S,k,L=c[m],C=L.key,z=L.nodeIndex,F=o[C],P=this.displayedNodeLabels.has(C),N=P?this.measureNodeLabel(F):{width:0,height:0,textHeight:0},G=N.textHeight,E=N.width,I=N.height,W=0,B=0;if(P&&F.labelAttachment&&l){var w=l.getEntry(C,F.labelAttachment);if(w){var $=F.labelAttachmentPlacement||"below";if($==="below"||$==="above"){var j=w.height/u;I+=j+yt,E=Math.max(E,w.width/u),B=$==="below"?(j+yt)/2:-(j+yt)/2}else{var J=w.width/u;E+=J+yt,I=Math.max(I,w.height/u),W=$==="right"?(J+yt)/2:-(J+yt)/2}}}var Se=F.backdropColor?pt(F.backdropColor):[255,255,255,255],le=F.backdropShadowColor?pt(F.backdropShadowColor):[0,0,0,128],Y=Se.map(function(X){return X/255}),Ke=le.map(function(X){return X/255}),we=(b=F.backdropShadowBlur)!==null&&b!==void 0?b:12,At=(x=F.backdropPadding)!==null&&x!==void 0?x:6,je=F.backdropBorderColor?pt(F.backdropBorderColor):[0,0,0,0],ia=je.map(function(X){return X/255}),Ht=(_=F.backdropBorderWidth)!==null&&_!==void 0?_:0,fe=(T=F.backdropCornerRadius)!==null&&T!==void 0?T:0,ae=(R=F.backdropLabelPadding)!==null&&R!==void 0?R:-1,ie=ae<0?At:ae,$e=(A=eg[(S=F.backdropArea)!==null&&S!==void 0?S:"both"])!==null&&A!==void 0?A:0,Q={key:C,nodeIndex:z,label:F.label,labelWidth:E,labelHeight:I,textHeight:G,type:"default",position:F.labelPosition||ut.labelPosition,labelAngle:(k=F.labelAngle)!==null&&k!==void 0?k:0,backdropColor:Y,backdropShadowColor:Ke,backdropShadowBlur:we,backdropPadding:At,backdropBorderColor:ia,backdropBorderWidth:Ht,backdropCornerRadius:fe,backdropLabelPadding:ie,backdropArea:$e,labelBoxOffset:[W,B]};i.processBackdrop(m,Q)}i.invalidateBuffers(),i.render(t)}}},{key:"renderLabelBackgrounds",value:function(t,r){var a=this.internals,i=a.labelBackgroundProgram,o=a.nodeDataCache,s=a.nodeDataTexture,l=ke.nodeLabel.writesPickingThisFrame(this.internals),u=[],d=U(this.displayedNodeLabels),c;try{for(d.s();!(c=d.n()).done;){var g=c.value,h=o[g];!h||h.visibility==="hidden"||r&&h.labelDepth!==r||!l&&!h.labelBackgroundColor||u.push(g)}}catch(z){d.e(z)}finally{d.f()}if(u.length!==0){i.reallocate(u.length);for(var f=0;f<u.length;f++){var y,v,p,m,b=u[f],x=o[b],_=(y=s?.getIndex(b))!==null&&y!==void 0?y:-1;if(!(_<0)){var T=this.measureNodeLabel(x),R=T.width,A=T.height,S=T.textHeight,k=Nt(this.internals.pickingState,"nodeLabel",b)||Nt(this.internals.pickingState,"node",b),L=x.labelBackgroundColor?We(x.labelBackgroundColor):We("transparent"),C={nodeIndex:_,id:en(k),color:L,labelWidth:R,labelHeight:A,textHeight:S,positionMode:(v=ln[x.labelPosition||ut.labelPosition])!==null&&v!==void 0?v:0,labelAngle:(p=x.labelAngle)!==null&&p!==void 0?p:0,padding:(m=x.labelBackgroundPadding)!==null&&m!==void 0?m:Ba};i.processLabelBackground(f,C)}}i.invalidateBuffers(),i.render(t)}}},{key:"cacheAttachments",value:function(t){var r=this.internals,a=r.attachmentManager,i=r.pixelRatio,o=r.nodeDataCache,s=r.nodesWithBackdrop,l=r.graph;if(a){var u=U(s),d;try{for(u.s();!(d=u.n()).done;){var c=d.value;if(this.displayedNodeLabels.has(c)){var g=o[c];if(!(!g||g.visibility==="hidden")&&!(t&&g.depth!==t)&&g.labelAttachment){var h=l.getNodeAttributes(c),f=this.measureNodeLabel(g),y=f.width,v=f.height,p={node:c,attributes:h,pixelRatio:i,labelWidth:y,labelHeight:v};a.renderAttachment(c,g.labelAttachment,p)}}}}catch(m){u.e(m)}finally{u.f()}a.regenerateAtlas()}}},{key:"renderAttachments",value:function(t,r){var a=this.internals,i=a.attachmentManager,o=a.attachmentProgram,s=a.nodeDataCache,l=a.nodesWithBackdrop,u=a.pixelRatio,d=a.nodeDataTexture;if(!(!i||!o)){var c=0;o.reallocateAttachments(l.size);var g=U(l),h;try{for(g.s();!(h=g.n()).done;){var f,y,v,p,m=h.value;if(this.displayedNodeLabels.has(m)){var b=s[m];if(!(!b||b.visibility==="hidden")&&!(r&&b.labelDepth!==r)&&b.labelAttachment){var x=i.getEntry(m,b.labelAttachment);if(x){var _=(f=d?.getIndex(m))!==null&&f!==void 0?f:-1;if(!(_<0)){var T=this.measureNodeLabel(b),R=T.width,A=T.height,S=T.textHeight,k=(y=Yo[b.labelAttachmentPlacement||"below"])!==null&&y!==void 0?y:0;o.processAttachment(c,{nodeIndex:_,atlasX:x.x,atlasY:x.y,atlasW:x.width,atlasH:x.height,attachWidth:x.width/u,attachHeight:x.height/u,positionMode:(v=ln[b.labelPosition||ut.labelPosition])!==null&&v!==void 0?v:0,attachmentPlacement:k,labelWidth:R,labelHeight:A,textHeight:S,labelAngle:(p=b.labelAngle)!==null&&p!==void 0?p:0}),c++}}}}}}catch(L){g.e(L)}finally{g.f()}c!==0&&(o.reallocateAttachments(c),i.bindTexture(In),o.invalidateBuffers(),o.render(t))}}},{key:"computeDisplayedEdgeLabels",value:function(){var t=this.internals,r=t.graph,a=t.stateManager,i=t.settings,o=t.edgesWithForcedLabels,s=a.getHighlightedNodes(),l=i.edgeLabelAnchors==="allNodes"?new Set(this.edgeAnchorGrid.getLabelsToDisplay(this.internals.getCameraState().ratio,i.labelDensity)):this.displayedNodeLabels,u=a.hovered,d=Qf({graph:r,hoveredNode:u?.kind==="node"?u.key:null,displayedNodeLabels:l,highlightedNodes:s});Da(d,o),this.edgeLabelCandidates=d,this.displayedEdgeLabels=new Set}},{key:"filterEdgeLabelsForDepth",value:function(t){for(var r=this.internals,a=r.graph,i=r.nodeDataCache,o=r.edgeDataCache,s=[],l=new Set,u=0,d=this.edgeLabelCandidates.length;u<d;u++){var c=this.edgeLabelCandidates[u];if(!l.has(c)){l.add(c);var g=a.extremities(c),h=i[g[0]],f=i[g[1]],y=o[c];!y||!h||!f||y.visibility==="hidden"||y.labelVisibility==="hidden"||h.visibility==="hidden"||f.visibility==="hidden"||t&&y.labelDepth!==t||y.label&&s.push(c)}}return s}},{key:"renderEdgeLabels",value:function(t,r){var a=this.internals,i=a.graph,o=a.nodeDataCache,s=a.edgeDataCache,l=a.primitives,u=a.edgeLabelProgram,d=a.nodeDataTexture,c=a.edgeDataTexture;if(u){var g=this.filterEdgeLabelsForDepth(r),h=U(g),f;try{for(h.s();!(f=h.n()).done;){var y=f.value;this.displayedEdgeLabels.add(y)}}catch(J){h.e(J)}finally{h.f()}if(g.length!==0){if(this.labelsDirty){var v,p,m=0,b=U(g),x;try{for(b.s();!(x=b.n()).done;){var _=x.value;m+=s[_].label.length}}catch(J){b.e(J)}finally{b.f()}u.reallocate(m);var T=(v=l==null||(p=l.edges)===null||p===void 0||(p=p.label)===null||p===void 0?void 0:p.margin)!==null&&v!==void 0?v:5,R="over",A=0,S=U(g),k;try{for(S.s();!(k=S.n()).done;){var L,C,z=k.value,F=i.extremities(z),P=F[0],N=F[1],G=o[P],E=o[N],I=s[z],W=d.getIndex(P),B=d.getIndex(N),w=c.getIndex(z),$={text:I.label,x:(G.x+E.x)/2,y:(G.y+E.y)/2,size:Fs,color:I.labelColor,nodeSize:0,nodeIndex:-1,margin:T,position:(L=I.labelPosition)!==null&&L!==void 0?L:R,hidden:!1,forceLabel:vt(I),type:"default",zIndex:(C=I.zIndex)!==null&&C!==void 0?C:0,parentType:"edge",parentKey:z,fontKey:"",labelAngle:0,sourceX:G.x,sourceY:G.y,targetX:E.x,targetY:E.y,sourceSize:G.size,targetSize:E.size,sourceShape:G.shape||"circle",targetShape:E.shape||"circle",edgeSize:I.size,offset:0,edgeAttributes:I,sourceNodeIndex:W,targetNodeIndex:B,edgeIndex:w},j=u.processEdgeLabel(z,A,$);A+=j}}catch(J){S.e(J)}finally{S.f()}u.invalidateBuffers()}u.render(t)}}}},{key:"renderEdgeLabelBackgrounds",value:function(t,r){var a,i,o=this.internals,s=o.edgeLabelBackgroundProgram,l=o.edgeLabelProgram,u=o.edgeDataCache,d=o.primitives,c=o.edgeDataTexture;if(!(!c||!l||!s)){var g=ke.edgeLabel.writesPickingThisFrame(this.internals),h=(a=d==null||(i=d.edges)===null||i===void 0||(i=i.label)===null||i===void 0?void 0:i.margin)!==null&&a!==void 0?a:5,f="over",y=this.filterEdgeLabelsForDepth(r),v=[],p=U(y),m;try{for(p.s();!(m=p.n()).done;){var b=m.value;!g&&!u[b].labelBackgroundColor||v.push(b)}}catch(G){p.e(G)}finally{p.f()}if(v.length!==0){s.reallocate(v.length);for(var x=0;x<v.length;x++){var _,T,R,A=v[x],S=u[A],k=S.label,L=l.measureLabelAtlasWidth(k),C=(_=S.labelPosition)!==null&&_!==void 0?_:f,z=typeof C=="string"&&(T=tg[C])!==null&&T!==void 0?T:0,F=Nt(this.internals.pickingState,"edgeLabel",A)||Nt(this.internals.pickingState,"edge",A),P=S.labelBackgroundColor?We(S.labelBackgroundColor):We("transparent"),N={edgeIndex:c.getIndex(A),baseFontSize:Fs,totalTextWidth:L,positionMode:z,margin:h,padding:(R=S.labelBackgroundPadding)!==null&&R!==void 0?R:ng,color:P,id:en(F),edgeAttributes:S};s.processEdgeLabelBackground(x,A,N)}s.invalidateBuffers(),s.render(t)}}}}])})(),ag=(function(){function e(n,t,r,a){ee(this,e),D(this,"nodeStates",new Map),D(this,"edgeStates",new Map),D(this,"hovered",null),D(this,"dirtyNodes",new Set),D(this,"dirtyEdges",new Set),D(this,"graphStateChanged",!1),D(this,"graphStateFlagsDirty",!1),this.scheduleRefresh=n,this.customNodeStateDefaults=t,this.customEdgeStateDefaults=r,this.customGraphStateDefaults=a,this.graphState=_a(a)}return te(e,[{key:"getNodeState",value:function(t){t=""+t;var r=this.nodeStates.get(t);return r||(r=yo(this.customNodeStateDefaults),this.nodeStates.set(t,r)),r}},{key:"getEdgeState",value:function(t){t=""+t;var r=this.edgeStates.get(t);return r||(r=_o(this.customEdgeStateDefaults),this.edgeStates.set(t,r)),r}},{key:"getGraphState",value:function(){return this.flushGraphStateFlags(),this.graphState}},{key:"getHighlightedNodes",value:function(){var t=new Set,r=U(this.nodeStates),a;try{for(r.s();!(a=r.n()).done;){var i=re(a.value,2),o=i[0],s=i[1];s.isHighlighted&&t.add(o)}}catch(l){r.e(l)}finally{r.f()}return t}},{key:"setNodeState",value:function(t,r){t=""+t;var a=this.getNodeState(t);if(rn(a,r)){var i=O(O({},a),r);this.nodeStates.set(t,i),this.dirtyNodes.add(t),this.updateHoveredNodeTracking(t,a,i),this.graphStateFlagsDirty=!0,this.scheduleRefresh()}}},{key:"setEdgeState",value:function(t,r){t=""+t;var a=this.getEdgeState(t);if(rn(a,r)){var i=O(O({},a),r);this.edgeStates.set(t,i),this.dirtyEdges.add(t),this.updateHoveredEdgeTracking(t,a,i),this.graphStateFlagsDirty=!0,this.scheduleRefresh()}}},{key:"setGraphState",value:function(t){if(rn(this.graphState,t)){var r=O(O({},this.graphState),t);r.isIdle=!r.isPanning&&!r.isZooming&&!r.isDragging,this.graphState=r,this.graphStateChanged=!0,this.scheduleRefresh()}}},{key:"setNodesState",value:function(t,r){var a=!1,i=U(t),o;try{for(i.s();!(o=i.n()).done;){var s=o.value;s=""+s;var l=this.getNodeState(s);if(rn(l,r)){var u=O(O({},l),r);this.nodeStates.set(s,u),this.dirtyNodes.add(s),this.updateHoveredNodeTracking(s,l,u),a=!0}}}catch(d){i.e(d)}finally{i.f()}a&&(this.graphStateFlagsDirty=!0,this.scheduleRefresh())}},{key:"setEdgesState",value:function(t,r){var a=!1,i=U(t),o;try{for(i.s();!(o=i.n()).done;){var s=o.value;s=""+s;var l=this.getEdgeState(s);if(rn(l,r)){var u=O(O({},l),r);this.edgeStates.set(s,u),this.dirtyEdges.add(s),this.updateHoveredEdgeTracking(s,l,u),a=!0}}}catch(d){i.e(d)}finally{i.f()}a&&(this.graphStateFlagsDirty=!0,this.scheduleRefresh())}},{key:"removeNode",value:function(t){this.nodeStates.delete(t),this.dirtyNodes.delete(t),this.clearHoveredFor("node",t)}},{key:"removeEdge",value:function(t){this.edgeStates.delete(t),this.dirtyEdges.delete(t),this.clearHoveredFor("edge",t)}},{key:"pruneNodes",value:function(t){var r=U(this.nodeStates.keys()),a;try{for(r.s();!(a=r.n()).done;){var i=a.value;t(i)||this.removeNode(i)}}catch(o){r.e(o)}finally{r.f()}}},{key:"pruneEdges",value:function(t){var r=U(this.edgeStates.keys()),a;try{for(r.s();!(a=r.n()).done;){var i=a.value;t(i)||this.removeEdge(i)}}catch(o){r.e(o)}finally{r.f()}}},{key:"clearNodes",value:function(){this.nodeStates.clear(),this.dirtyNodes.clear(),this.clearHoveredForKinds(Ja("node"))}},{key:"clearEdges",value:function(){this.edgeStates.clear(),this.dirtyEdges.clear(),this.clearHoveredForKinds(Ja("edge"))}},{key:"resetGraphState",value:function(){this.graphState=_a(this.customGraphStateDefaults),this.graphStateChanged=!1,this.graphStateFlagsDirty=!1}},{key:"clearDirtyTracking",value:function(){this.dirtyNodes.clear(),this.dirtyEdges.clear(),this.graphStateChanged=!1}},{key:"setHovered",value:function(t){this.hovered=t}},{key:"clearHoveredFor",value:function(t,r){this.hovered&&this.hovered.key===r&&Ja(t).includes(this.hovered.kind)&&(this.hovered=null)}},{key:"clearHoveredForKinds",value:function(t){this.hovered&&t.includes(this.hovered.kind)&&(this.hovered=null)}},{key:"updateGraphStateFromNodes",value:function(){var t,r=!1,a=!1,i=!1,o=U(this.nodeStates),s;try{for(o.s();!(s=o.n()).done;){var l=re(s.value,2),u=l[1];if(u.isHovered&&(r=!0),u.isHighlighted&&(a=!0),u.isDragged&&(i=!0),r&&a&&i)break}}catch(c){o.e(c)}finally{o.f()}!r&&((t=this.hovered)===null||t===void 0?void 0:t.kind)==="edge"&&(r=!0);var d=!this.graphState.isPanning&&!this.graphState.isZooming&&!i;(this.graphState.hasHovered!==r||this.graphState.hasHighlighted!==a||this.graphState.isDragging!==i||this.graphState.isIdle!==d)&&(this.graphStateChanged=!0),this.graphState=O(O({},this.graphState),{},{hasHovered:r,hasHighlighted:a,isDragging:i,isIdle:d})}},{key:"updateGraphStateFromEdges",value:function(){var t,r=((t=this.hovered)===null||t===void 0?void 0:t.kind)==="node";if(!r){var a=U(this.edgeStates),i;try{for(a.s();!(i=a.n()).done;){var o=re(i.value,2),s=o[1];if(s.isHovered){r=!0;break}}}catch(l){a.e(l)}finally{a.f()}}this.graphState.hasHovered!==r&&(this.graphStateChanged=!0),this.graphState=O(O({},this.graphState),{},{hasHovered:r})}},{key:"flushGraphStateFlags",value:function(){this.graphStateFlagsDirty&&(this.updateGraphStateFromNodes(),this.updateGraphStateFromEdges(),this.graphStateFlagsDirty=!1)}},{key:"updateHoveredNodeTracking",value:function(t,r,a){var i;if(r.isHovered!==a.isHovered)if(a.isHovered){var o;if(((o=this.hovered)===null||o===void 0?void 0:o.kind)==="node"&&this.hovered.key!==t){var s=this.hovered.key,l=this.getNodeState(s);this.nodeStates.set(s,O(O({},l),{},{isHovered:!1})),this.dirtyNodes.add(s)}this.hovered={kind:"node",key:t}}else((i=this.hovered)===null||i===void 0?void 0:i.kind)==="node"&&this.hovered.key===t&&(this.hovered=null)}},{key:"updateHoveredEdgeTracking",value:function(t,r,a){var i;if(r.isHovered!==a.isHovered)if(a.isHovered){var o;if(((o=this.hovered)===null||o===void 0?void 0:o.kind)==="edge"&&this.hovered.key!==t){var s=this.hovered.key,l=this.getEdgeState(s);this.edgeStates.set(s,O(O({},l),{},{isHovered:!1})),this.dirtyEdges.add(s)}this.hovered={kind:"edge",key:t}}else((i=this.hovered)===null||i===void 0?void 0:i.kind)==="edge"&&this.hovered.key===t&&(this.hovered=null)}}])})(),Ls=1,Ps=2,Is=3,Gs=4,ig=(function(e){function n(t,r){var a,i,o,s,l,u=arguments.length>2&&arguments[2]!==void 0?arguments[2]:{};ee(this,n),l=ge(this,n),D(l,"nodeReducer",null),D(l,"edgeReducer",null),D(l,"stageCanvas",null),D(l,"mouseLayer",null),D(l,"gestureHint",null),D(l,"extraElements",{}),D(l,"webGLContext",null),D(l,"pickingFrameBuffer",null),D(l,"pickingTexture",null),D(l,"pickingDepthBuffer",null),D(l,"activeListeners",{}),D(l,"nodeVariableEntries",[]),D(l,"edgeVariableEntries",[]),D(l,"edgePathsByName",new Map),D(l,"nodeProgramIndex",{}),D(l,"edgeProgramIndex",{}),D(l,"edgeTextureIndexCache",{}),D(l,"nodeGraphCoords",{}),D(l,"nodeExtent",{x:[0,1],y:[0,1]}),D(l,"matrix",Me()),D(l,"invMatrix",Me()),D(l,"correctionRatio",1),D(l,"frameId",0),D(l,"customBBox",null),D(l,"gpuTimerExt",void 0),D(l,"activeGpuTimerQuery",null),D(l,"pendingGpuTimerQueries",[]),D(l,"normalizationFunction",wr({x:[0,1],y:[0,1]})),D(l,"graphToViewportRatio",1),D(l,"pickingState",Pf()),D(l,"prevNodeVisibilities",{}),D(l,"width",0),D(l,"height",0),D(l,"autoRescaleFrozen",!1),D(l,"stylesDeclaration",null),D(l,"resolvedStageStyle",{}),D(l,"renderFrame",null),D(l,"contextLost",!1),D(l,"pendingProcess","full"),D(l,"needToRefreshState",!1),D(l,"checkEdgesEventsFrame",null),D(l,"edgeStyleAnalysis",{dependency:"static",xAttribute:null,yAttribute:null}),D(l,"depthLayers",K(xr)),D(l,"customLayerPrograms",new Map),D(l,"nodeShapeSlug",null),D(l,"sdfAtlas",null),D(l,"depthRanges",{nodes:{},edges:{}}),D(l,"nodeBaseDepth",{}),D(l,"edgeBaseDepth",{});var d=u.primitives,c=u.styles,g=u.settings,h=g===void 0?{}:g,f=u.nodeReducer,y=u.edgeReducer,v=u.customNodeState,p=u.customEdgeState,m=u.customGraphState;l.stateManager=new ag(function(){return l.scheduleStateRefresh()},v,p,m);var b=d??Eo;l.stylesDeclaration=c?{nodes:(a=c.nodes)!==null&&a!==void 0?a:Sr.nodes,edges:(i=c.edges)!==null&&i!==void 0?i:Sr.edges,stage:c.stage}:Sr,l.nodeReducer=f??null,l.edgeReducer=y??null;var x=xa(l.stylesDeclaration.nodes);l.nodeReducer&&(x.dependency="graph-state"),l.edgeStyleAnalysis=xa(l.stylesDeclaration.edges),l.edgeReducer&&(l.edgeStyleAnalysis.dependency="graph-state"),l.stylesDeclaration.stage&&(l.resolvedStageStyle=Aa(l.stylesDeclaration.stage,l.stateManager.graphState));var _=Co(h);if(Cr(_),_.enableNodeDrag){var T=x.xAttribute,R=x.yAttribute;if((!T||!R)&&!_.dragPositionToAttributes)throw new Error('Sigma: `enableNodeDrag` is true but position attribute names could not be inferred from styles. Either use attribute bindings for x/y in your node styles (e.g. `x: { attribute: "x" }`), or provide a `dragPositionToAttributes` setting.')}if(ko(t),!(r instanceof HTMLElement))throw new Error("Sigma: container should be an html element.");l.container=r,l.edgeGroups=new Lf(t,function(E,I){for(var W=0;W<E.length;W++){var B=l.stateManager.getEdgeState(E[W]);B.parallelIndex=W,B.parallelCount=I}});var A=new Ff(t,l.viewportToGraph.bind(l),l.setNodesState.bind(l),function(E,I){return l.emit(E,I)});if(l.depthLayers=(o=b.depthLayers)!==null&&o!==void 0?o:K(xr),!(c!=null&&c.nodes)&&!yr.every(function(E){return l.depthLayers.includes(E)}))throw new Error("Sigma: depthLayers must include ".concat(yr.join(", ")," for the built-in node styles."));if(!(c!=null&&c.edges)&&!_r.every(function(E){return l.depthLayers.includes(E)}))throw new Error("Sigma: depthLayers must include ".concat(_r.join(", ")," for the built-in edge styles."));l.itemBuckets={nodes:new Va(l.depthLayers),edges:new Va(l.depthLayers)},l.initWebGLContext(),l.mouseLayer=on("div",{position:"absolute",touchAction:"none",userSelect:"none"},{class:"sigma-mouse"}),l.container.appendChild(l.mouseLayer),l.resolvedStageStyle.background&&(l.container.style.backgroundColor=l.resolvedStageStyle.background),l.resolvedStageStyle.cursor&&(l.container.style.cursor=l.resolvedStageStyle.cursor);var S=new ms(l.webGLContext),k=new qa(l.webGLContext,{channels:1}),L=new bs(l.webGLContext),C=new qa(l.webGLContext,{channels:4}),z=l.webGLContext,F=new Zf({gl:z,getFrameBuffer:function(){return l.pickingFrameBuffer},getPixelRatio:function(){return l.internals.pixelRatio},getDownSizingRatio:function(){return l.internals.settings.pickingDownSizingRatio},isRenderPending:function(){return l.renderFrame!==null},onIndex:function(I,W){var B;return Ms(l.internals,(B=l.pickingState.lookup[I])!==null&&B!==void 0?B:null,W)}}),P=l.initPrograms(b,_),N=b==null||(s=b.nodes)===null||s===void 0?void 0:s.labelAttachments,G=null;return N&&Object.keys(N).length>0&&(G=new vs(z,N,function(){return l.scheduleRender()})),l.internals=O(O({nodeDataCache:{},edgeDataCache:{},nodesWithForcedLabels:new Set,nodesWithBackdrop:new Set,edgesWithForcedLabels:new Set,settings:_,primitives:b,pixelRatio:Dn(),graph:t,stateManager:l.stateManager,dragManager:A,hoverResolver:F,nodeStyleAnalysis:x,pickingState:l.pickingState},P),{},{attachmentManager:G,nodeDataTexture:S,nodeFrameTexture:k,edgeDataTexture:L,edgeFrameTexture:C,getDimensions:function(){return l.getDimensions()},getGraphDimensions:function(){return l.getGraphDimensions()},getStagePadding:function(){return l.getStagePadding()},getCameraState:function(){return l.camera.getState()},getHitAtPosition:function(I){return l.getHitAtPosition(I)},setNodeState:function(I,W){return l.setNodeState(I,W)},setEdgeState:function(I,W){return l.setEdgeState(I,W)},updateContainerCursor:function(){return l.updateContainerCursor()},scheduleRefresh:function(){return l.scheduleRefresh()},viewportToFramedGraph:function(I){return l.viewportToFramedGraph(I)},viewportToGraph:function(I){return l.viewportToGraph(I)},framedGraphToViewport:function(I){return l.framedGraphToViewport(I)},scaleSize:function(I){return l.scaleSize(I)},emit:function(I,W){return l.emit(I,W)}}),l.labelRenderer=new rg(l.internals),l.resize(),l.initializeWebGLLabels(),l.camera=new xs,l.bindCameraHandlers(),l.mouseCaptor=new Df(l.mouseLayer,l),l.mouseCaptor.setSettings(l.internals.settings),l.touchCaptor=new Cf(l.mouseLayer,l),l.touchCaptor.setSettings(l.internals.settings),l.bindEventHandlers(),l.bindGraphHandlers(),l.handleSettingsUpdate(),l.refresh(),l}return pe(n,e),te(n,[{key:"initializeWebGLLabels",value:function(){this.sdfAtlas=new Pt,this.sdfAtlas.registerFont({family:"sans-serif",weight:"normal",style:"normal"})}},{key:"resetWebGLTexture",value:function(){var r=this.webGLContext;if(!this.pickingFrameBuffer)return this;var a=Math.ceil(this.width*this.internals.pixelRatio/this.internals.settings.pickingDownSizingRatio),i=Math.ceil(this.height*this.internals.pixelRatio/this.internals.settings.pickingDownSizingRatio);r.bindFramebuffer(r.FRAMEBUFFER,this.pickingFrameBuffer),this.pickingTexture&&r.deleteTexture(this.pickingTexture);var o=r.createTexture();o&&(r.bindTexture(r.TEXTURE_2D,o),r.texImage2D(r.TEXTURE_2D,0,r.RGBA,a,i,0,r.RGBA,r.UNSIGNED_BYTE,null),r.texParameteri(r.TEXTURE_2D,r.TEXTURE_MIN_FILTER,r.NEAREST),r.texParameteri(r.TEXTURE_2D,r.TEXTURE_MAG_FILTER,r.NEAREST),r.framebufferTexture2D(r.FRAMEBUFFER,r.COLOR_ATTACHMENT0,r.TEXTURE_2D,o,0),this.pickingTexture=o),this.pickingDepthBuffer&&r.deleteRenderbuffer(this.pickingDepthBuffer);var s=r.createRenderbuffer();return s&&(r.bindRenderbuffer(r.RENDERBUFFER,s),r.renderbufferStorage(r.RENDERBUFFER,r.DEPTH_COMPONENT16,a,i),r.framebufferRenderbuffer(r.FRAMEBUFFER,r.DEPTH_ATTACHMENT,r.RENDERBUFFER,s),this.pickingDepthBuffer=s),r.bindFramebuffer(r.FRAMEBUFFER,null),this}},{key:"bindCameraHandlers",value:function(){var r=this;return this.activeListeners.camera=function(){r.refreshMatrices(),r.labelRenderer.labelsDirty=!0,r.scheduleRender()},this.activeListeners.cameraAnimationStart=function(a){var i=a.from,o=a.to;i.ratio!==o.ratio&&r.stateManager.setGraphState({isZooming:!0})},this.activeListeners.cameraAnimationEnd=function(){r.stateManager.setGraphState({isZooming:!1})},this.camera.on("updated",this.activeListeners.camera),this.camera.on("animationStart",this.activeListeners.cameraAnimationStart),this.camera.on("animationEnd",this.activeListeners.cameraAnimationEnd),this}},{key:"unbindCameraHandlers",value:function(){return this.camera.removeListener("updated",this.activeListeners.camera),this.camera.removeListener("animationStart",this.activeListeners.cameraAnimationStart),this.camera.removeListener("animationEnd",this.activeListeners.cameraAnimationEnd),this}},{key:"getHitAtPosition",value:function(r){var a;if(this.contextLost)return null;var i=this.webGLContext;i.bindFramebuffer(i.FRAMEBUFFER,this.pickingFrameBuffer);var o=Zi(i,this.pickingFrameBuffer,r.x,r.y,this.internals.pixelRatio,this.internals.settings.pickingDownSizingRatio),s=pa.apply(void 0,K(o));return(a=this.pickingState.lookup[s])!==null&&a!==void 0?a:null}},{key:"bindEventHandlers",value:function(){return Wf(this.internals,this.mouseCaptor,this.touchCaptor,this.activeListeners),this}},{key:"bindGraphHandlers",value:function(){return Uf({graph:this.internals.graph,edgeGroups:this.edgeGroups,addNode:this.addNode.bind(this),updateNode:this.updateNode.bind(this),removeNode:this.removeNode.bind(this),addEdge:this.addEdge.bind(this),updateEdge:this.updateEdge.bind(this),removeEdge:this.removeEdge.bind(this),clearEdgeState:this.clearEdgeState.bind(this),clearNodeState:this.clearNodeState.bind(this),clearEdgeIndices:this.clearEdgeIndices.bind(this),clearNodeIndices:this.clearNodeIndices.bind(this),refresh:this.refresh.bind(this)},this.activeListeners),this}},{key:"unbindGraphHandlers",value:function(){Of(this.internals.graph,this.activeListeners)}},{key:"getNodeShapeId",value:function(r){return this.internals.nodeShapeMap&&this.internals.nodeGlobalShapeIds&&r.shape&&r.shape in this.internals.nodeShapeMap?this.internals.nodeGlobalShapeIds[this.internals.nodeShapeMap[r.shape]]:Ln(r.shape||"circle")}},{key:"processNodes",value:function(){var r=this,a=this.internals.graph,i=this.internals.settings,o=this.getDimensions(),s=i.autoRescale,l=i.autoRescaleContent,u=this.nodeExtent;if(s===!1){var d=o.width,c=o.height;u={x:[-d/2,d/2],y:[-c/2,c/2]}}else(s!=="once"||!this.autoRescaleFrozen)&&(u=this.computeNodeExtent(),l!=="positions"&&!this.customBBox&&(u=Vf({extent:u,coords:this.nodeGraphCoords,nodeData:this.internals.nodeDataCache,dimensions:o,stagePadding:this.getStagePadding(),zoomToSizeRatioFunction:i.zoomToSizeRatioFunction,itemSizesReference:i.itemSizesReference,fitLabels:l==="labels",nodeLabelBox:function(B,w){return r.labelRenderer.nodeLabelBox(B,w)}})),s==="once"&&(this.autoRescaleFrozen=!0));this.nodeExtent=u,this.normalizationFunction=wr(this.customBBox||this.nodeExtent);var g=new xs,h=gt(g.getState(),o,this.getGraphDimensions(),this.getStagePadding());this.labelRenderer.labelGrid.resizeAndClear(o,i.labelGridCellSize),this.labelRenderer.edgeAnchorGrid.resizeAndClear(o,i.labelGridCellSize);var f=i.renderEdgeLabels&&i.edgeLabelAnchors==="allNodes",y=!1;Ya(this.pickingState,"node");for(var v=a.nodes(),p=0,m=v.length;p<m;p++){var b=v[p],x=this.internals.nodeDataCache[b],_=this.nodeGraphCoords[b];x.x=_.x,x.y=_.y,this.normalizationFunction.applyTo(x),x.visibility!==this.prevNodeVisibilities[b]&&(y=!0),typeof x.label=="string"&&x.visibility!=="hidden"&&x.labelVisibility!=="hidden"&&this.labelRenderer.labelGrid.add(b,x.size,this.framedGraphToViewport(x,{matrix:h})),f&&x.visibility!=="hidden"&&this.labelRenderer.edgeAnchorGrid.add(b,x.size,this.framedGraphToViewport(x,{matrix:h}))}this.labelRenderer.labelGrid.organize(),this.labelRenderer.edgeAnchorGrid.organize(),this.nodeProgram.reallocate(v.length);var T=0;this.depthRanges.nodes={},this.nodeBaseDepth={};var R=this.internals.nodeDataCache,A=U(this.depthLayers),S;try{for(A.s();!(S=A.n()).done;){var k=S.value,L=this.itemBuckets.nodes.getSorted(k,function(W){return R[W].zIndex});if(L.length!==0){this.depthRanges.nodes[k]=[{offset:T,count:L.length}];var C=U(L),z;try{for(C.s();!(z=C.n()).done;){var F=z.value;this.nodeBaseDepth[F]=k,this.nodeProgram.allocateNode(F),As(this.pickingState,"node",F),this.addNodeToProgram(F,T++)}}catch(W){C.e(W)}finally{C.f()}}}}catch(W){A.e(W)}finally{A.f()}this.nodeProgram.invalidateBuffers();for(var P=0,N=v.length;P<N;P++)this.prevNodeVisibilities[v[P]]=this.internals.nodeDataCache[v[P]].visibility;this.labelRenderer.processWebGLLabels(v);var G=U(this.customLayerPrograms.values()),E;try{for(G.s();!(E=G.n()).done;){var I=E.value.program;I.cacheData&&I.cacheData()}}catch(W){G.e(W)}finally{G.f()}return y}},{key:"processEdges",value:function(){var r=this.internals.graph,a=r.edges();this.edgeProgram.reallocate(a.length);var i=0;Ya(this.pickingState,"edge"),this.depthRanges.edges={},this.edgeBaseDepth={};var o=this.internals.edgeDataCache,s=U(this.depthLayers),l;try{for(s.s();!(l=s.n()).done;){var u=l.value,d=this.itemBuckets.edges.getSorted(u,function(f){return o[f].zIndex});if(d.length!==0){this.depthRanges.edges[u]=[{offset:i,count:d.length}];var c=U(d),g;try{for(c.s();!(g=c.n()).done;){var h=g.value;this.edgeBaseDepth[h]=u,As(this.pickingState,"edge",h),this.addEdgeToProgram(h,i++)}}catch(f){c.e(f)}finally{c.f()}}}}catch(f){s.e(f)}finally{s.f()}this.edgeProgram.invalidateBuffers()}},{key:"updateNodeDepthRanges",value:function(r,a,i){var o=this.nodeProgramIndex[r];o!==void 0&&(ka(this.depthRanges.nodes,a,o),Ca(this.depthRanges.nodes,i,o))}},{key:"updateEdgeDepthRanges",value:function(r,a,i){var o=this.edgeProgramIndex[r];o!==void 0&&(ka(this.depthRanges.edges,a,o),Ca(this.depthRanges.edges,i,o))}},{key:"handleSettingsUpdate",value:function(){var r=this,a=this.internals.settings;return this.camera.minRatio=a.minCameraRatio,this.camera.maxRatio=a.maxCameraRatio,this.camera.enabledZooming=a.enableCameraZooming,this.camera.enabledPanning=a.enableCameraPanning,this.camera.enabledRotation=a.enableCameraRotation,a.cameraPanBoundaries?this.camera.constrainState=function(i){return r.cleanCameraState(i,a.cameraPanBoundaries&&ne(a.cameraPanBoundaries)==="object"?a.cameraPanBoundaries:{})}:this.camera.constrainState=null,this.camera.setState(this.camera.getState()),this.mouseLayer.style.touchAction=a.gestureTarget==="graph"?"none":a.gestureTarget==="shared"?"pan-x pan-y":"auto",a.gestureTarget!=="shared"&&this.gestureHint&&(this.gestureHint.kill(),this.gestureHint=null),this.mouseCaptor.setSettings(this.internals.settings),this.touchCaptor.setSettings(this.internals.settings),this}},{key:"cleanCameraState",value:function(r){var a=arguments.length>1&&arguments[1]!==void 0?arguments[1]:{},i=a.tolerance,o=i===void 0?0:i,s=a.boundaries,l=O({},r),u=s||this.nodeExtent,d=re(u.x,2),c=d[0],g=d[1],h=re(u.y,2),f=h[0],y=h[1],v=[this.graphToViewport({x:c,y:f},{cameraState:r}),this.graphToViewport({x:g,y:f},{cameraState:r}),this.graphToViewport({x:c,y},{cameraState:r}),this.graphToViewport({x:g,y},{cameraState:r})],p=1/0,m=-1/0,b=1/0,x=-1/0;v.forEach(function(F){var P=F.x,N=F.y;p=Math.min(p,P),m=Math.max(m,P),b=Math.min(b,N),x=Math.max(x,N)});var _=m-p,T=x-b,R=this.getDimensions(),A=R.width,S=R.height,k=0,L=0;if(_>=A?m<A-o?k=m-(A-o):p>o&&(k=p-o):m>A+o?k=m-(A+o):p<-o&&(k=p+o),T>=S?x<S-o?L=x-(S-o):b>o&&(L=b-o):x>S+o?L=x-(S+o):b<-o&&(L=b+o),k||L){var C=this.viewportToFramedGraph({x:0,y:0},{cameraState:r}),z=this.viewportToFramedGraph({x:k,y:L},{cameraState:r});k=z.x-C.x,L=z.y-C.y,l.x+=k,l.y+=L}return l}},{key:"refreshMatrices",value:function(){var r=this.camera.getState(),a=this.getDimensions(),i=this.getGraphDimensions(),o=this.getStagePadding();this.matrix=gt(r,a,i,o),this.invMatrix=gt(r,a,i,o,!0),this.correctionRatio=Xi(this.matrix,r,a),this.graphToViewportRatio=this.getGraphToViewportRatio()}},{key:"processData",value:function(){var r;this.emit("beforeProcess"),(r=this.internals.attachmentManager)===null||r===void 0||r.clear();var a=O({},this.pickingState.idsByKind),i=this.processNodes();(this.pendingProcess==="full"||i)&&this.processEdges(),If(this.pickingState,this.internals),Gf(a,this.pickingState.idsByKind)||this.internals.hoverResolver.invalidate(),this.pendingProcess="none",this.emit("afterProcess")}},{key:"render",value:function(){var r=this;if(this.contextLost)return this;this.emit("beforeRender");var a=function(){return r.internals.hoverResolver.frameRendered(),r.emit("afterRender"),r};if(this.renderFrame&&(cancelAnimationFrame(this.renderFrame),this.renderFrame=null),this.resize(),(this.pendingProcess!=="none"||this.needToRefreshState)&&(this.labelRenderer.labelsDirty=!0),this.pendingProcess!=="none"&&this.processData(),this.needToRefreshState&&this.refreshState(),this.needToRefreshState=!1,this.stateManager.clearDirtyTracking(),this.pendingProcess!=="none"&&this.processData(),this.clear(),this.resetWebGLTexture(),!this.internals.graph.order)return a();var i=this.mouseCaptor,o=this.camera.isAnimating()||i.isMoving||i.draggedEvents||i.currentWheelDirection;if(this.refreshMatrices(),this.frameId++,this.internals.settings.renderEdgeLabels&&!this.internals.edgeLabelProgram){var s=this.createEdgeLabelPrograms(),l=s.labelProgram,u=s.labelBackgroundProgram;this.internals.edgeLabelProgram=l,this.internals.edgeLabelBackgroundProgram=u}var d=this.internals.settings.DEBUG_logRenderStats?this.getDebugPrograms():null;d&&d.forEach(function(J){return J.resetDebugStats()}),this.internals.settings.DEBUG_gpuTimerQueries&&this.beginGpuTimerQuery(),this.internals.nodeFrameTexture.ensureCapacity(this.internals.nodeDataTexture.getCapacity()),this.internals.edgeFrameTexture.ensureCapacity(this.internals.edgeDataTexture.getCapacity());var c=this.getRenderParams(),g=ke.edge.writesPickingThisFrame(this.internals)?c:O(O({},c),{},{pickingFrameBuffer:null});this.labelRenderer.resetFrame();var h=this.webGLContext,f=Math.ceil(this.width*this.internals.pixelRatio/this.internals.settings.pickingDownSizingRatio),y=Math.ceil(this.height*this.internals.pixelRatio/this.internals.settings.pickingDownSizingRatio);if(h.bindFramebuffer(h.FRAMEBUFFER,this.pickingFrameBuffer),h.viewport(0,0,f,y),h.clear(h.COLOR_BUFFER_BIT|h.DEPTH_BUFFER_BIT),h.bindFramebuffer(h.FRAMEBUFFER,null),h.viewport(0,0,this.width*this.internals.pixelRatio,this.height*this.internals.pixelRatio),h.clear(h.COLOR_BUFFER_BIT|h.DEPTH_BUFFER_BIT),this.internals.nodeDataTexture.upload(),this.internals.edgeDataTexture.upload(),this.nodeProgram.uploadLayerTexture(),this.edgeProgram.uploadAttributeTexture(),this.emit("afterTexturesUpload"),this.internals.nodeDataTexture.bind(Is),this.internals.edgeDataTexture.bind(Gs),this.edgeFramePass.run(c,this.internals.edgeFrameTexture,this.internals.edgeDataTexture.getHighWaterMark(),this.edgeProgram.getAttributeTexture(),this.nodeProgram.getAttributeTexture()),this.internals.edgeFrameTexture.bind(Ls),this.internals.settings.renderLabels){this.labelRenderer.computeDisplayedNodeLabels();var v=this.labelRenderer.buildFramePassPoints(),p=v.data,m=v.count;this.nodeFramePass.run(p,m,this.internals.nodeFrameTexture,c,this.nodeProgram.getAttributeTexture()),this.internals.nodeFrameTexture.bind(Ps)}this.internals.settings.renderEdgeLabels&&this.labelRenderer.computeDisplayedEdgeLabels();var b=U(this.customLayerPrograms.values()),x;try{for(b.s();!(x=b.n()).done;){var _=x.value.program;_.preRender&&_.preRender(c)}}catch(J){b.e(J)}finally{b.f()}h.bindFramebuffer(h.FRAMEBUFFER,null),h.viewport(0,0,this.width*this.internals.pixelRatio,this.height*this.internals.pixelRatio),h.blendFunc(h.ONE,h.ONE_MINUS_SRC_ALPHA);var T=!this.internals.settings.hideLabelsOnMove||!o,R=U(this.depthLayers),A;try{for(R.s();!(A=R.n()).done;){var S=A.value,k=U(this.customLayerPrograms.values()),L;try{for(k.s();!(L=k.n()).done;){var C=L.value;C.depth===S&&C.program.render(c)}}catch(J){k.e(J)}finally{k.f()}var z=this.depthRanges.edges[S];if(z&&(!this.internals.settings.hideEdgesOnMove||!o)){var F=U(z),P;try{for(F.s();!(P=F.n()).done;){var N=P.value,G=N.offset,E=N.count;E>0&&this.edgeProgram.render(g,G,E)}}catch(J){F.e(J)}finally{F.f()}}this.internals.settings.renderEdgeLabels&&T&&(this.labelRenderer.renderEdgeLabelBackgrounds(ke.edgeLabel.writesPickingThisFrame(this.internals)?c:O(O({},c),{},{pickingFrameBuffer:null}),S),this.labelRenderer.renderEdgeLabels(c,S)),this.labelRenderer.cacheAttachments(S),this.labelRenderer.renderBackdrops(O(O({},c),{},{pickingFrameBuffer:null}),S);var I=this.depthRanges.nodes[S];if(I){var W=U(I),B;try{for(W.s();!(B=W.n()).done;){var w=B.value,$=w.offset,j=w.count;j>0&&this.nodeProgram.render(c,$,j)}}catch(J){W.e(J)}finally{W.f()}}T&&(this.labelRenderer.renderAttachments(O(O({},c),{},{pickingFrameBuffer:null}),S),this.labelRenderer.renderLabelBackgrounds(ke.nodeLabel.writesPickingThisFrame(this.internals)?c:O(O({},c),{},{pickingFrameBuffer:null}),S),this.internals.settings.renderLabels&&this.labelRenderer.renderWebGLLabels(c,S))}}catch(J){R.e(J)}finally{R.f()}return this.labelRenderer.labelsDirty=!1,this.internals.settings.DEBUG_displayPickingLayer&&(h.bindFramebuffer(h.READ_FRAMEBUFFER,this.pickingFrameBuffer),h.bindFramebuffer(h.DRAW_FRAMEBUFFER,null),h.blitFramebuffer(0,0,f,y,0,0,this.width*this.internals.pixelRatio,this.height*this.internals.pixelRatio,h.COLOR_BUFFER_BIT,h.NEAREST)),this.internals.settings.DEBUG_gpuTimerQueries&&this.endGpuTimerQuery(),this.pollGpuTimerQueries(),d&&this.logRenderStats(d),a()}},{key:"getDebugPrograms",value:function(){return[this.nodeProgram,this.edgeProgram,this.internals.labelProgram,this.internals.edgeLabelProgram,this.internals.edgeLabelBackgroundProgram,this.internals.backdropProgram,this.internals.labelBackgroundProgram,this.internals.attachmentProgram].filter(function(r){return r!==null})}},{key:"logRenderStats",value:function(r){var a={},i=U(r),o;try{for(i.s();!(o=i.n()).done;){var s=o.value;a[s.constructor.name]=O({},s.debugStats)}}catch(l){i.e(l)}finally{i.f()}console.log("[sigma] DEBUG_logRenderStats: frame #".concat(this.frameId)),console.table(a)}},{key:"getGpuTimerExtension",value:function(){return this.gpuTimerExt===void 0&&(this.gpuTimerExt=this.webGLContext.getExtension("EXT_disjoint_timer_query_webgl2"),this.gpuTimerExt||console.warn("Sigma: DEBUG_gpuTimerQueries is enabled, but this browser/driver doesn't support EXT_disjoint_timer_query_webgl2.")),this.gpuTimerExt}},{key:"beginGpuTimerQuery",value:function(){var r=this.getGpuTimerExtension();if(r){var a=this.webGLContext,i=a.createQuery();i&&(a.beginQuery(r.TIME_ELAPSED_EXT,i),this.activeGpuTimerQuery=i)}}},{key:"endGpuTimerQuery",value:function(){!this.gpuTimerExt||!this.activeGpuTimerQuery||(this.webGLContext.endQuery(this.gpuTimerExt.TIME_ELAPSED_EXT),this.pendingGpuTimerQueries.push({query:this.activeGpuTimerQuery,frameId:this.frameId}),this.activeGpuTimerQuery=null)}},{key:"pollGpuTimerQueries",value:function(){if(this.pendingGpuTimerQueries.length!==0){var r=this.webGLContext,a=this.gpuTimerExt;if(!a){this.pendingGpuTimerQueries.forEach(function(c){var g=c.query;return r.deleteQuery(g)}),this.pendingGpuTimerQueries=[];return}var i=r.getParameter(a.GPU_DISJOINT_EXT),o=[],s=U(this.pendingGpuTimerQueries),l;try{for(s.s();!(l=s.n()).done;){var u=l.value;if(!r.getQueryParameter(u.query,r.QUERY_RESULT_AVAILABLE)){o.push(u);continue}if(!i){var d=r.getQueryParameter(u.query,r.QUERY_RESULT);console.log("[sigma] DEBUG_gpuTimerQueries: frame #".concat(u.frameId," GPU time = ").concat((d/1e6).toFixed(2),"ms"))}r.deleteQuery(u.query)}}catch(c){s.e(c)}finally{s.f()}this.pendingGpuTimerQueries=o}}},{key:"postEvaluateNode",value:function(r,a,i){var o=r;o.x===void 0&&(o.x=a.x),o.y===void 0&&(o.y=a.y),r.highlighted=i.isHighlighted;for(var s=0,l=this.nodeVariableEntries.length;s<l;s++){var u,d,c=re(this.nodeVariableEntries[s],2),g=c[0],h=c[1];o[g]=(u=(d=o[g])!==null&&d!==void 0?d:a[g])!==null&&u!==void 0?u:h.default}}},{key:"addNode",value:function(r){var a=this.internals.graph.getNodeAttributes(r),i=this.stateManager.getNodeState(r),o=this.internals.nodeDataCache[r]||{};if(Sa(this.stylesDeclaration.nodes,a,i,this.stateManager.graphState,this.internals.graph,o),this.postEvaluateNode(o,a,i),this.nodeReducer){var s=this.nodeReducer(r,o,a,i,this.stateManager.graphState,this.internals.graph);o=O(O({},o),s)}if(typeof o.x!="number"||typeof o.y!="number")throw new Error('Sigma: could not find a valid position (x, y) for node "'.concat(r,'". ')+"Provide coordinates via node attributes, styles, or a nodeReducer.");this.internals.nodeShapeMap?(!o.shape||!(o.shape in this.internals.nodeShapeMap))&&(o.shape=Object.keys(this.internals.nodeShapeMap)[0]):this.nodeShapeSlug&&(o.shape=this.nodeShapeSlug),this.internals.nodeDataCache[r]=o,this.nodeGraphCoords[r]={x:o.x,y:o.y},Lt(this.internals.nodesWithForcedLabels,r,vt(o)),Lt(this.internals.nodesWithBackdrop,r,wa(o)),this.itemBuckets.nodes.set(r,o.depth)}},{key:"updateNode",value:function(r){this.addNode(r);var a=this.internals.nodeDataCache[r];this.normalizationFunction.applyTo(a)}},{key:"removeNode",value:function(r){this.itemBuckets.nodes.remove(r),delete this.internals.nodeDataCache[r],delete this.nodeGraphCoords[r],delete this.nodeProgramIndex[r],this.internals.dragManager.removeNode(r),this.stateManager.removeNode(r),this.internals.nodesWithForcedLabels.delete(r),this.internals.nodesWithBackdrop.delete(r)}},{key:"postEvaluateEdge",value:function(r,a){for(var i=r,o=0,s=this.edgeVariableEntries.length;o<s;o++){var l,u,d=re(this.edgeVariableEntries[o],2),c=d[0],g=d[1];i[c]=(l=(u=i[c])!==null&&u!==void 0?u:a[c])!==null&&l!==void 0?l:g.default}}},{key:"applyEdgeSpread",value:function(r,a,i){var o;if(!(i.parallelCount<=1)){var s=this.internals.graph.source(r),l=this.internals.graph.target(r),u=s===l,d=u?a.selfLoopPath||a.path:a.parallelPath||a.path,c=d?this.edgePathsByName.get(d):void 0;if(c!=null&&c.spread){var g=(o=a.parallelSpread)!==null&&o!==void 0?o:.25,h=c.spread.compute(i.parallelIndex,i.parallelCount,g);!u&&this.internals.graph.isDirected(r)&&s>l&&(h=-h),a[c.spread.variable]=h}}}},{key:"addEdge",value:function(r){var a=this.internals.graph.getEdgeAttributes(r),i=this.stateManager.getEdgeState(r),o={};if(Ra(this.stylesDeclaration.edges,a,i,this.stateManager.graphState,this.internals.graph,o),this.postEvaluateEdge(o,a),this.edgeReducer){var s=this.edgeReducer(r,o,a,i,this.stateManager.graphState,this.internals.graph);o=O(O({},o),s)}this.applyEdgeSpread(r,o,i),this.internals.edgeDataCache[r]=o,Lt(this.internals.edgesWithForcedLabels,r,vt(o)),this.itemBuckets.edges.set(r,o.depth)}},{key:"updateEdge",value:function(r){this.addEdge(r)}},{key:"removeEdge",value:function(r){this.itemBuckets.edges.remove(r),delete this.internals.edgeDataCache[r],delete this.edgeProgramIndex[r],delete this.edgeTextureIndexCache[r],this.internals.edgeDataTexture.free(r),this.stateManager.removeEdge(r),this.internals.edgesWithForcedLabels.delete(r)}},{key:"clearNodeIndices",value:function(){this.labelRenderer.resetLabelGrid(),this.autoRescaleFrozen||(this.nodeExtent={x:[0,1],y:[0,1]}),this.internals.nodeDataCache={},this.nodeGraphCoords={},this.edgeProgramIndex={},this.internals.nodesWithForcedLabels.clear(),this.internals.nodesWithBackdrop.clear(),this.prevNodeVisibilities={},this.itemBuckets.nodes.clearAll(),this.depthRanges.nodes={},this.nodeBaseDepth={}}},{key:"clearEdgeIndices",value:function(){this.internals.edgeDataCache={},this.edgeProgramIndex={},this.edgeTextureIndexCache={},this.internals.edgesWithForcedLabels.clear(),Ya(this.pickingState,"edge"),this.itemBuckets.edges.clearAll(),this.depthRanges.edges={},this.edgeBaseDepth={},this.edgeGroups.clear()}},{key:"clearIndices",value:function(){this.clearEdgeIndices(),this.clearNodeIndices()}},{key:"clearNodeState",value:function(){this.labelRenderer.resetFrame(),this.internals.nodesWithBackdrop.clear(),this.internals.dragManager.clear(),this.autoRescaleFrozen=!1,this.stateManager.clearNodes()}},{key:"clearEdgeState",value:function(){this.labelRenderer.clearEdgeLabels(),this.stateManager.clearEdges()}},{key:"clearState",value:function(){this.clearEdgeState(),this.clearNodeState(),this.stateManager.resetGraphState()}},{key:"addNodeToProgram",value:function(r,a){var i,o=this.internals.nodeDataCache[r];this.internals.nodeDataTexture.allocate(r),(i=this.internals.nodeDataTexture).updateNode.apply(i,[r,o.x,o.y,o.size,this.getNodeShapeId(o)].concat(K(Dr(o)),[o.color]));var s=this.internals.nodeDataTexture.getIndex(r);this.nodeProgram.process(Nt(this.pickingState,"node",r),a,o,s,r),this.nodeProgramIndex[r]=a}},{key:"addEdgeToProgram",value:function(r,a){var i,o,s=this.internals.edgeDataCache[r],l=this.internals.graph.source(r),u=this.internals.graph.target(r),d=this.internals.edgeDataTexture.allocate(r);this.edgeTextureIndexCache[r]=d;var c=l===u,g=!c&&((i=(o=this.stateManager.getEdgeState(r))===null||o===void 0?void 0:o.parallelCount)!==null&&i!==void 0?i:1)>1,h=this.edgeProgram.resolveEdgeIds(s,c,g),f=h.pathId,y=h.headId,v=h.tailId,p=h.headLengthRatio,m=h.tailLengthRatio;this.internals.edgeDataTexture.updateEdge(r,this.internals.nodeDataTexture.getIndex(l),this.internals.nodeDataTexture.getIndex(u),s.size,p,m,f,y,v),this.edgeProgram.process(Nt(this.pickingState,"edge",r),a,this.internals.nodeDataCache[l],this.internals.nodeDataCache[u],s,d),this.edgeProgramIndex[r]=a}},{key:"getRenderParams",value:function(){return{frameId:this.frameId,matrix:this.matrix,invMatrix:this.invMatrix,width:this.width,height:this.height,pixelRatio:this.internals.pixelRatio,zoomRatio:this.camera.ratio,cameraAngle:this.camera.angle,sizeRatio:1/this.scaleSize(),correctionRatio:this.correctionRatio,downSizingRatio:this.internals.settings.pickingDownSizingRatio,minEdgeThickness:this.internals.settings.minEdgeThickness,antiAliasingFeather:this.internals.settings.antiAliasingFeather,nodePickingPadding:this.internals.settings.nodePickingPadding,edgePickingPadding:this.internals.settings.edgePickingPadding,labelPickingPadding:this.internals.settings.labelPickingPadding,nodeDataTextureUnit:Is,nodeDataTextureWidth:this.internals.nodeDataTexture.getTextureWidth(),nodeFrameTextureUnit:Ps,nodeFrameTextureWidth:this.internals.nodeFrameTexture.getTextureWidth(),edgeDataTextureUnit:Gs,edgeDataTextureWidth:this.internals.edgeDataTexture.getTextureWidth(),edgeFrameTextureUnit:Ls,edgeFrameTextureWidth:this.internals.edgeFrameTexture.getTextureWidth(),pickingFrameBuffer:this.pickingFrameBuffer,labelPixelSnapping:this.internals.settings.labelPixelSnapping?1:0}}},{key:"getStagePadding",value:function(){var r=this.internals.settings,a=r.stagePadding,i=r.autoRescale;return i&&a||0}},{key:"getLayerElement",value:function(r){if(r==="mouse")return this.mouseLayer;var a=this.extraElements[r];if(!a)throw new Error('Sigma: layer "'.concat(r,'" does not exist'));return a}},{key:"initWebGLContext",value:function(){var r=this,a=this.createWebGLContext("stage");this.stageCanvas=this.extraElements.stage,this.webGLContext=a,this.stageCanvas.addEventListener("webglcontextlost",function(i){r.webGLContext&&(i.preventDefault(),r.contextLost=!0,r.renderFrame&&(cancelAnimationFrame(r.renderFrame),r.renderFrame=null),r.gpuTimerExt=void 0,r.activeGpuTimerQuery=null,r.pendingGpuTimerQueries=[],r.emit("webglContextLost"))}),this.stageCanvas.addEventListener("webglcontextrestored",function(){requestAnimationFrame(function(){r.webGLContext&&!r.webGLContext.isContextLost()&&r.restoreWebGLContext()})}),this.initPickingFramebuffer()}},{key:"initPickingFramebuffer",value:function(){var r=this.webGLContext,a=r.createFramebuffer();if(!a)throw new Error("Sigma: cannot create picking frame buffer");r.bindFramebuffer(r.FRAMEBUFFER,a);var i=r.createTexture();if(!i)throw new Error("Sigma: cannot create picking texture");r.bindTexture(r.TEXTURE_2D,i),r.texImage2D(r.TEXTURE_2D,0,r.RGBA,1,1,0,r.RGBA,r.UNSIGNED_BYTE,null),r.texParameteri(r.TEXTURE_2D,r.TEXTURE_MIN_FILTER,r.NEAREST),r.texParameteri(r.TEXTURE_2D,r.TEXTURE_MAG_FILTER,r.NEAREST),r.texParameteri(r.TEXTURE_2D,r.TEXTURE_WRAP_S,r.CLAMP_TO_EDGE),r.texParameteri(r.TEXTURE_2D,r.TEXTURE_WRAP_T,r.CLAMP_TO_EDGE),r.framebufferTexture2D(r.FRAMEBUFFER,r.COLOR_ATTACHMENT0,r.TEXTURE_2D,i,0);var o=r.createRenderbuffer();if(!o)throw new Error("Sigma: cannot create picking depth buffer");if(r.bindRenderbuffer(r.RENDERBUFFER,o),r.renderbufferStorage(r.RENDERBUFFER,r.DEPTH_COMPONENT16,1,1),r.framebufferRenderbuffer(r.FRAMEBUFFER,r.DEPTH_ATTACHMENT,r.RENDERBUFFER,o),r.checkFramebufferStatus(r.FRAMEBUFFER)!==r.FRAMEBUFFER_COMPLETE)throw new Error("Sigma: picking framebuffer is not complete");r.bindFramebuffer(r.FRAMEBUFFER,null),this.pickingFrameBuffer=a,this.pickingTexture=i,this.pickingDepthBuffer=o}},{key:"initPrograms",value:function(r,a){var i=this,o=this.webGLContext,s=cs(o,this.pickingFrameBuffer,i,r?.nodes,a.antialiasNodes),l=s.nodeProgram,u=s.labelProgram,d=s.backdropProgram,c=s.labelBackgroundProgram,g=s.attachmentProgram,h=s.framePass,f=s.shapeSlug,y=s.shapeNameToIndex,v=s.shapeGlobalIds,p=s.variables;this.nodeProgram=l,this.nodeFramePass=h,this.nodeVariableEntries=Object.entries(p),f&&(this.nodeShapeSlug=f);var m=ds(o,this.pickingFrameBuffer,i,r?.edges,a.antialiasEdges,r?.nodes),b=m.edgeProgram,x=m.createLabelPrograms,_=m.framePass,T=m.variables,R=m.paths;return this.createEdgeLabelPrograms=x,this.edgeProgram=b,this.edgeFramePass=_,this.edgeVariableEntries=Object.entries(T),this.edgePathsByName=new Map(R.map(function(A){return[A.name,A]})),{labelProgram:u,edgeLabelProgram:null,edgeLabelBackgroundProgram:null,backdropProgram:d,labelBackgroundProgram:c,attachmentProgram:g,nodeShapeMap:y??null,nodeGlobalShapeIds:v??null}}},{key:"restoreWebGLContext",value:function(){var r,a,i,o,s,l=this.internals;this.initPickingFramebuffer(),(r=l.nodeDataTexture)===null||r===void 0||r.restore(),(a=l.nodeFrameTexture)===null||a===void 0||a.restore(),(i=l.edgeDataTexture)===null||i===void 0||i.restore(),(o=l.edgeFrameTexture)===null||o===void 0||o.restore(),(s=l.attachmentManager)===null||s===void 0||s.restore(),l.hoverResolver.reset(),Object.assign(l,this.initPrograms(l.primitives,l.settings));var u=U(this.customLayerPrograms.values()),d;try{for(u.s();!(d=u.n()).done;){var c=d.value;c.program.kill(),c.program=c.factory(this.webGLContext)}}catch(g){u.e(g)}finally{u.f()}this.contextLost=!1,this.emit("webglContextRestored"),this.refresh()}},{key:"createLayer",value:function(r,a){var i=arguments.length>2&&arguments[2]!==void 0?arguments[2]:{};if(this.extraElements[r])throw new Error('Sigma: a layer named "'.concat(r,'" already exists'));var o=on(a,{position:"absolute"},{class:"sigma-".concat(r)});return i.style&&Object.assign(o.style,i.style),this.extraElements[r]=o,"beforeLayer"in i&&i.beforeLayer?this.getLayerElement(i.beforeLayer).before(o):"afterLayer"in i&&i.afterLayer?this.getLayerElement(i.afterLayer).after(o):this.container.appendChild(o),o}},{key:"createCanvas",value:function(r){var a=arguments.length>1&&arguments[1]!==void 0?arguments[1]:{};return this.createLayer(r,"canvas",a)}},{key:"createWebGLContext",value:function(r){var a=arguments.length>1&&arguments[1]!==void 0?arguments[1]:{},i=this.createCanvas(r,a);a.hidden&&i.remove();var o=i.getContext("webgl2",O({preserveDrawingBuffer:!1,antialias:!1,depth:!0},a));if(!o)throw new Error("Sigma: WebGL 2 is not supported by your browser. Please use a modern browser (Chrome 56+, Firefox 51+, Safari 15+, Edge 79+).");return o.blendFunc(o.ONE,o.ONE_MINUS_SRC_ALPHA),o}},{key:"killLayer",value:function(r){if(r==="stage"||r==="mouse")throw new Error('Sigma: cannot kill built-in layer "'.concat(r,'"'));var a=this.extraElements[r];if(!a)throw new Error("Sigma: cannot kill layer ".concat(r,", which does not exist"));return a.remove(),delete this.extraElements[r],this}},{key:"getWebGLContext",value:function(){if(!this.webGLContext)throw new Error("Sigma: WebGL context is not available");return this.webGLContext}},{key:"getNodeDataTexture",value:function(){if(!this.internals.nodeDataTexture)throw new Error("Sigma: node data texture is not available");return this.internals.nodeDataTexture}},{key:"getNormalizationFunction",value:function(){return this.normalizationFunction}},{key:"addCustomLayerProgram",value:function(r,a,i){var o;if(!this.depthLayers.includes(a))throw new Error('Sigma: cannot add custom layer program at depth "'.concat(a,'", ')+"it must be declared in primitives.depthLayers. Current layers: ".concat(this.depthLayers.join(", ")));(o=this.customLayerPrograms.get(r))===null||o===void 0||o.program.kill();var s=i(this.webGLContext);return this.customLayerPrograms.set(r,{depth:a,factory:i,program:s}),s.cacheData&&s.cacheData(),this.scheduleRender(),this}},{key:"removeCustomLayerProgram",value:function(r){var a=this.customLayerPrograms.get(r);return a&&(a.program.kill(),this.customLayerPrograms.delete(r),this.scheduleRender()),this}},{key:"getCamera",value:function(){return this.camera}},{key:"setCamera",value:function(r){return this.camera.cancelAnimation(),this.unbindCameraHandlers(),this.camera=r,this.bindCameraHandlers(),this.scheduleRender(),this}},{key:"getContainer",value:function(){return this.container}},{key:"getGraph",value:function(){return this.internals.graph}},{key:"setGraph",value:function(r){return r===this.internals.graph?this:(this.stateManager.pruneNodes(function(a){return r.hasNode(a)}),this.stateManager.pruneEdges(function(a){return r.hasEdge(a)}),this.unbindGraphHandlers(),this.checkEdgesEventsFrame!==null&&(cancelAnimationFrame(this.checkEdgesEventsFrame),this.checkEdgesEventsFrame=null),this.internals.graph=r,this.autoRescaleFrozen=!1,this.bindGraphHandlers(),this.refresh(),this)}},{key:"getMouseCaptor",value:function(){return this.mouseCaptor}},{key:"getTouchCaptor",value:function(){return this.touchCaptor}},{key:"getDimensions",value:function(){return{width:this.width,height:this.height}}},{key:"getGraphDimensions",value:function(){var r=this.customBBox||this.nodeExtent;return{width:r.x[1]-r.x[0]||1,height:r.y[1]-r.y[0]||1}}},{key:"getNodeDisplayData",value:function(r){var a=this.internals.nodeDataCache[r];return a?Object.assign({},a):void 0}},{key:"getEdgeDisplayData",value:function(r){var a=this.internals.edgeDataCache[r];return a?Object.assign({},a):void 0}},{key:"getNodeState",value:function(r){return this.stateManager.getNodeState(r)}},{key:"getEdgeState",value:function(r){return this.stateManager.getEdgeState(r)}},{key:"getGraphState",value:function(){return this.stateManager.getGraphState()}},{key:"setNodeState",value:function(r,a){return this.stateManager.setNodeState(r,a),this}},{key:"setEdgeState",value:function(r,a){return this.stateManager.setEdgeState(r,a),this}},{key:"setGraphState",value:function(r){return this.stateManager.setGraphState(r),this}},{key:"_setPanning",value:function(r){this.stateManager.setGraphState({isPanning:r})}},{key:"_setZooming",value:function(r){this.stateManager.setGraphState({isZooming:r})}},{key:"_hasNodeDrag",value:function(){var r=this.internals.dragManager;return!!(r.pendingNode||r.session)}},{key:"_showGestureHint",value:function(r){var a=this.internals.settings;a.gestureTarget==="shared"&&(this.gestureHint=this.gestureHint||new Kf(this.container),this.gestureHint.show(r,a))}},{key:"setNodesState",value:function(r,a){return this.stateManager.setNodesState(r,a),this}},{key:"setEdgesState",value:function(r,a){return this.stateManager.setEdgesState(r,a),this}},{key:"updateContainerCursor",value:function(){var r=this.stateManager.hovered,a=this.resolvedStageStyle.cursor||"";this.container.style.cursor=r&&zf(this.internals,r)||a}},{key:"refreshStageStyle",value:function(){this.resolvedStageStyle=Aa(this.stylesDeclaration.stage,this.stateManager.graphState),this.resolvedStageStyle.background!==void 0&&(this.container.style.backgroundColor=this.resolvedStageStyle.background),this.updateContainerCursor()}},{key:"getNodeDisplayedLabels",value:function(){return new Set(this.labelRenderer.displayedNodeLabels)}},{key:"getEdgeDisplayedLabels",value:function(){return new Set(this.labelRenderer.displayedEdgeLabels)}},{key:"getSettings",value:function(){return O({},this.internals.settings)}},{key:"getSetting",value:function(r){return this.internals.settings[r]}},{key:"getStyles",value:function(){return O({},this.stylesDeclaration)}},{key:"getPrimitives",value:function(){return O({},this.internals.primitives)}},{key:"setSetting",value:function(r,a){return this.internals.settings[r]=a,Cr(this.internals.settings),this.handleSettingsUpdate(),this.scheduleRefresh(),this}},{key:"updateSetting",value:function(r,a){return this.setSetting(r,a(this.internals.settings[r])),this}},{key:"setSettings",value:function(r){return this.internals.settings=O(O({},this.internals.settings),r),Cr(this.internals.settings),this.handleSettingsUpdate(),this.scheduleRefresh(),this}},{key:"resize",value:function(r){var a=this.width,i=this.height;if(this.width=this.container.offsetWidth,this.height=this.container.offsetHeight,this.internals.pixelRatio=Dn(),this.width===0)if(this.internals.settings.allowInvalidContainer)this.width=1;else throw new Error("Sigma: Container has no width. You can set the allowInvalidContainer setting to true to stop seeing this error.");if(this.height===0)if(this.internals.settings.allowInvalidContainer)this.height=1;else throw new Error("Sigma: Container has no height. You can set the allowInvalidContainer setting to true to stop seeing this error.");if(!r&&a===this.width&&i===this.height)return this;this.labelRenderer.labelsDirty=!0;for(var o=0,s=[this.mouseLayer].concat(K(Object.values(this.extraElements)));o<s.length;o++){var l=s[o];l.style.width=this.width+"px",l.style.height=this.height+"px"}return this.webGLContext&&(this.stageCanvas.setAttribute("width",this.width*this.internals.pixelRatio+"px"),this.stageCanvas.setAttribute("height",this.height*this.internals.pixelRatio+"px"),this.webGLContext.viewport(0,0,this.width*this.internals.pixelRatio,this.height*this.internals.pixelRatio)),this.emit("resize"),this}},{key:"clear",value:function(){return this.emit("beforeClear"),this.webGLContext.bindFramebuffer(WebGLRenderingContext.FRAMEBUFFER,null),this.webGLContext.clear(WebGLRenderingContext.COLOR_BUFFER_BIT),this.emit("afterClear"),this}},{key:"scheduleStateRefresh",value:function(){this.needToRefreshState=!0,this.scheduleRender()}},{key:"refreshState",value:function(){var r=this,a;this.stateManager.flushGraphStateFlags();var i=this.stateManager.graphStateChanged&&this.internals.nodeStyleAnalysis.dependency==="graph-state",o=this.stateManager.graphStateChanged&&this.edgeStyleAnalysis.dependency==="graph-state",s=!1;if(i)this.internals.graph.forEachNode(function(f){r.refreshNodeState(f)&&(s=!0)});else if(this.internals.nodeStyleAnalysis.dependency!=="static"){var l=U(this.stateManager.dirtyNodes),u;try{for(l.s();!(u=l.n()).done;){var d=u.value;this.refreshNodeState(d)&&(s=!0)}}catch(f){l.e(f)}finally{l.f()}}if(o)this.internals.graph.forEachEdge(function(f){r.refreshEdgeState(f)&&(s=!0)});else if(this.edgeStyleAnalysis.dependency!=="static"){var c=U(this.stateManager.dirtyEdges),g;try{for(c.s();!(g=c.n()).done;){var h=g.value;this.refreshEdgeState(h)&&(s=!0)}}catch(f){c.e(f)}finally{c.f()}}this.stateManager.graphStateChanged&&(a=this.stylesDeclaration)!==null&&a!==void 0&&a.stage&&this.refreshStageStyle(),this.stateManager.clearDirtyTracking(),s&&(this.pendingProcess="full")}},{key:"refreshNodeState",value:function(r){var a=this.internals.nodeDataCache[r];if(!a||this.nodeReducer){var i,o,s,l,u=(i=this.internals.nodeDataCache[r])===null||i===void 0?void 0:i.depth,d=(o=this.internals.nodeDataCache[r])===null||o===void 0?void 0:o.zIndex,c=(s=this.internals.nodeDataCache[r])===null||s===void 0?void 0:s.labelAttachment;this.updateNode(r);var g=this.internals.nodeDataCache[r];this.internals.attachmentManager&&g.labelAttachment!==c&&this.internals.attachmentManager.invalidateNode(r);var h;this.internals.nodeShapeMap&&this.internals.nodeGlobalShapeIds&&g.shape&&g.shape in this.internals.nodeShapeMap?h=this.internals.nodeGlobalShapeIds[this.internals.nodeShapeMap[g.shape]]:h=Ln(g.shape||"circle"),(l=this.internals.nodeDataTexture).updateNode.apply(l,[r,g.x,g.y,g.size,h].concat(K(Dr(g)),[g.color])),u&&g.depth!==u&&this.updateNodeDepthRanges(r,u,g.depth);var f=this.nodeProgramIndex[r];return f!==void 0&&(this.addNodeToProgram(r,f),this.nodeProgram.invalidateBuffers()),d!==void 0&&g.zIndex!==d}var y=this.internals.graph.getNodeAttributes(r),v=this.stateManager.getNodeState(r),p=a.size,m=a.shape,b=a.depth,x=a.zIndex,_=a.labelAttachment,T=a.rotationAlignment,R=a.labelRotationAlignment,A=a.color;Sa(this.stylesDeclaration.nodes,y,v,this.stateManager.graphState,this.internals.graph,a),this.postEvaluateNode(a,y,v);var S=this.nodeGraphCoords[r],k=a.x!==S.x||a.y!==S.y;if(k&&(S.x=a.x,S.y=a.y),this.normalizationFunction.applyTo(a),this.internals.nodeShapeMap?(!a.shape||!(a.shape in this.internals.nodeShapeMap))&&(a.shape=Object.keys(this.internals.nodeShapeMap)[0]):this.nodeShapeSlug&&(a.shape=this.nodeShapeSlug),this.internals.attachmentManager&&a.labelAttachment!==_&&this.internals.attachmentManager.invalidateNode(r),Lt(this.internals.nodesWithForcedLabels,r,vt(a)),Lt(this.internals.nodesWithBackdrop,r,wa(a)),k||a.size!==p||a.shape!==m||a.rotationAlignment!==T||a.labelRotationAlignment!==R||a.color!==A){var L,C;this.internals.nodeShapeMap&&this.internals.nodeGlobalShapeIds&&a.shape&&a.shape in this.internals.nodeShapeMap?C=this.internals.nodeGlobalShapeIds[this.internals.nodeShapeMap[a.shape]]:C=Ln(a.shape||"circle"),(L=this.internals.nodeDataTexture).updateNode.apply(L,[r,a.x,a.y,a.size,C].concat(K(Dr(a)),[a.color]))}this.itemBuckets.nodes.set(r,a.depth),a.depth!==b&&this.updateNodeDepthRanges(r,b,a.depth);var z=this.nodeProgramIndex[r];return z!==void 0&&(this.addNodeToProgram(r,z),this.nodeProgram.invalidateBuffers()),a.zIndex!==x}},{key:"refreshEdgeState",value:function(r){var a=this.internals.edgeDataCache[r];if(!a||this.edgeReducer){var i=a?.depth,o=a?.zIndex;this.updateEdge(r);var s=this.internals.edgeDataCache[r];i&&s.depth!==i&&this.updateEdgeDepthRanges(r,i,s.depth);var l=this.edgeProgramIndex[r];return l!==void 0&&(this.addEdgeToProgram(r,l),this.edgeProgram.invalidateBuffers()),o!==void 0&&s.zIndex!==o}var u=this.internals.graph.getEdgeAttributes(r),d=this.stateManager.getEdgeState(r),c=a.depth,g=a.zIndex,h=a.size,f=a.path,y=a.selfLoopPath,v=a.parallelPath,p=a.head,m=a.tail;Ra(this.stylesDeclaration.edges,u,d,this.stateManager.graphState,this.internals.graph,a),this.postEvaluateEdge(a,u),this.applyEdgeSpread(r,a,d),Lt(this.internals.edgesWithForcedLabels,r,vt(a)),this.itemBuckets.edges.set(r,a.depth),a.depth!==c&&this.updateEdgeDepthRanges(r,c,a.depth);var b=this.edgeProgramIndex[r];if(b!==void 0){var x=a.size!==h||a.path!==f||a.selfLoopPath!==y||a.parallelPath!==v||a.head!==p||a.tail!==m;if(x)this.addEdgeToProgram(r,b),this.edgeProgram.invalidateBuffers();else{var _=this.internals.graph.source(r),T=this.internals.graph.target(r),R=this.internals.nodeDataCache[_],A=this.internals.nodeDataCache[T],S=this.edgeTextureIndexCache[r];this.edgeProgram.process(Nt(this.pickingState,"edge",r),b,R,A,a,S),this.edgeProgram.invalidateBuffers()}}return a.zIndex!==g}},{key:"refresh",value:function(r){var a=this,i=r?.skipIndexation!==void 0?r?.skipIndexation:!1,o=r?.schedule!==void 0?r.schedule:!1,s=!r||!r.partialGraph;if(s)this.clearEdgeIndices(),this.clearNodeIndices(),this.internals.graph.forEachNode(function(R){return a.addNode(R)}),this.edgeGroups.rebuild(),this.internals.graph.forEachEdge(function(R){return a.addEdge(R)}),this.pendingProcess="full";else{for(var l,u,d=((l=r.partialGraph)===null||l===void 0?void 0:l.nodes)||[],c=0,g=d?.length||0;c<g;c++){var h,f,y=""+d[c],v=(h=this.internals.nodeDataCache[y])===null||h===void 0?void 0:h.labelAttachment;if(this.updateNode(y),this.internals.attachmentManager&&((f=this.internals.nodeDataCache[y])!==null&&f!==void 0&&f.labelAttachment||v)&&this.internals.attachmentManager.invalidateNode(y),i){var p=this.nodeProgramIndex[y];if(p===void 0)throw new Error('Sigma: node "'.concat(y,`" can't be repaint`));this.addNodeToProgram(y,p)}}i&&d.length>0&&(this.nodeProgram.invalidateBuffers(),this.labelRenderer.labelsDirty=!0);for(var m=(r==null||(u=r.partialGraph)===null||u===void 0?void 0:u.edges)||[],b=0,x=m.length;b<x;b++){var _=""+m[b];if(this.updateEdge(_),i){var T=this.edgeProgramIndex[_];if(T===void 0)throw new Error('Sigma: edge "'.concat(_,`" can't be repaint`));this.addEdgeToProgram(_,T)}}i&&m.length>0&&(this.edgeProgram.invalidateBuffers(),this.labelRenderer.labelsDirty=!0),!i&&this.pendingProcess!=="full"&&(this.pendingProcess=m.length>0?"full":"nodes")}return o?this.scheduleRender():this.render(),this}},{key:"scheduleRender",value:function(){var r=this;return this.renderFrame||(this.renderFrame=requestAnimationFrame(function(){r.render()})),this}},{key:"scheduleRefresh",value:function(r){return this.refresh(O(O({},r),{},{schedule:!0}))}},{key:"getViewportZoomedState",value:function(r,a){var i=this.camera.getState(),o=i.ratio,s=i.angle,l=i.x,u=i.y,d=this.internals.settings,c=d.minCameraRatio,g=d.maxCameraRatio;typeof g=="number"&&(a=Math.min(a,g)),typeof c=="number"&&(a=Math.max(a,c));var h=a/o,f={x:this.width/2,y:this.height/2},y=this.viewportToFramedGraph(r),v=this.viewportToFramedGraph(f);return{angle:s,x:(y.x-v.x)*(1-h)+l,y:(y.y-v.y)*(1-h)+u,ratio:a}}},{key:"viewRectangle",value:function(){var r=this.viewportToFramedGraph({x:0,y:0}),a=this.viewportToFramedGraph({x:this.width,y:0}),i=this.viewportToFramedGraph({x:0,y:this.height});return{x1:r.x,y1:r.y,x2:a.x,y2:a.y,height:a.y-i.y}}},{key:"framedGraphToViewport",value:function(r){var a=arguments.length>1&&arguments[1]!==void 0?arguments[1]:{},i=!!a.cameraState||!!a.viewportDimensions||!!a.graphDimensions||!!a.padding,o=a.matrix||(i?gt(a.cameraState||this.camera.getState(),a.viewportDimensions||this.getDimensions(),a.graphDimensions||this.getGraphDimensions(),a.padding||this.getStagePadding()):this.matrix),s=Qt(o,r);return{x:(1+s.x)*this.width/2,y:(1-s.y)*this.height/2}}},{key:"viewportToFramedGraph",value:function(r){var a=arguments.length>1&&arguments[1]!==void 0?arguments[1]:{},i=!!a.cameraState||!!a.viewportDimensions||!!a.graphDimensions||!!a.padding,o=a.matrix||(i?gt(a.cameraState||this.camera.getState(),a.viewportDimensions||this.getDimensions(),a.graphDimensions||this.getGraphDimensions(),a.padding||this.getStagePadding(),!0):this.invMatrix),s=Qt(o,{x:r.x/this.width*2-1,y:1-r.y/this.height*2});return isNaN(s.x)&&(s.x=0),isNaN(s.y)&&(s.y=0),s}},{key:"viewportToGraph",value:function(r){var a=arguments.length>1&&arguments[1]!==void 0?arguments[1]:{};return this.normalizationFunction.inverse(this.viewportToFramedGraph(r,a))}},{key:"graphToViewport",value:function(r){var a=arguments.length>1&&arguments[1]!==void 0?arguments[1]:{};return this.framedGraphToViewport(this.normalizationFunction(r),a)}},{key:"graphToFramedGraph",value:function(r){return this.normalizationFunction(r)}},{key:"framedGraphToGraph",value:function(r){return this.normalizationFunction.inverse(r)}},{key:"getGraphToViewportRatio",value:function(){var r={x:0,y:0},a={x:1,y:1},i=Math.sqrt(Math.pow(r.x-a.x,2)+Math.pow(r.y-a.y,2)),o=this.graphToViewport(r),s=this.graphToViewport(a),l=Math.sqrt(Math.pow(o.x-s.x,2)+Math.pow(o.y-s.y,2));return l/i}},{key:"computeNodeExtent",value:function(){var r=this.nodeGraphCoords,a=Object.keys(r);if(!a.length)return{x:[0,1],y:[0,1]};for(var i=1/0,o=-1/0,s=1/0,l=-1/0,u=0,d=a.length;u<d;u++){var c=r[a[u]],g=c.x,h=c.y;g<i&&(i=g),g>o&&(o=g),h<s&&(s=h),h>l&&(l=h)}return{x:[i,o],y:[s,l]}}},{key:"getBBox",value:function(){return this.nodeExtent}},{key:"getCustomBBox",value:function(){return this.customBBox}},{key:"setCustomBBox",value:function(r){return this.customBBox=r,this.scheduleRender(),this}},{key:"kill",value:function(){var r,a,i,o,s;this.emit("kill"),this.removeAllListeners(),this.camera.cancelAnimation(),this.unbindCameraHandlers(),window.removeEventListener("resize",this.activeListeners.handleResize),this.mouseCaptor.kill(),this.touchCaptor.kill(),this.internals.hoverResolver.kill(),this.unbindGraphHandlers(),this.clearIndices(),this.clearState(),this.internals.nodeDataCache={},this.internals.edgeDataCache={},this.renderFrame&&(cancelAnimationFrame(this.renderFrame),this.renderFrame=null);for(var l=this.container;l.firstChild;)l.removeChild(l.firstChild);this.nodeProgram.kill(),this.nodeFramePass.kill(),this.edgeProgram.kill(),this.edgeFramePass.kill(),this.internals.labelProgram.kill(),(r=this.internals.edgeLabelProgram)===null||r===void 0||r.kill(),(a=this.internals.edgeLabelBackgroundProgram)===null||a===void 0||a.kill(),this.internals.backdropProgram.kill(),this.internals.labelBackgroundProgram.kill(),(i=this.internals.attachmentProgram)===null||i===void 0||i.kill(),(o=this.internals.attachmentManager)===null||o===void 0||o.kill(),this.internals.attachmentProgram=null,this.internals.attachmentManager=null;var u=U(this.customLayerPrograms.values()),d;try{for(u.s();!(d=u.n()).done;){var c=d.value.program;c.kill()}}catch(f){u.e(f)}finally{u.f()}if(this.customLayerPrograms.clear(),this.sdfAtlas&&(this.sdfAtlas=null),this.internals.nodeDataTexture&&(this.internals.nodeDataTexture.kill(),this.internals.nodeDataTexture=null),this.internals.nodeFrameTexture&&(this.internals.nodeFrameTexture.kill(),this.internals.nodeFrameTexture=null),this.internals.edgeDataTexture&&(this.internals.edgeDataTexture.kill(),this.internals.edgeDataTexture=null),this.internals.edgeFrameTexture&&(this.internals.edgeFrameTexture.kill(),this.internals.edgeFrameTexture=null),this.webGLContext){var g;(g=this.webGLContext.getExtension("WEBGL_lose_context"))===null||g===void 0||g.loseContext(),this.webGLContext=null}(s=this.gestureHint)===null||s===void 0||s.kill(),this.gestureHint=null,this.mouseLayer.remove();for(var h in this.extraElements)this.extraElements[h].remove();this.extraElements={}}},{key:"scaleSize",value:function(){var r=arguments.length>0&&arguments[0]!==void 0?arguments[0]:1,a=arguments.length>1&&arguments[1]!==void 0?arguments[1]:this.camera.ratio;return r/this.internals.settings.zoomToSizeRatioFunction(a)*(this.getSetting("itemSizesReference")==="positions"?a*this.graphToViewportRatio:1)}},{key:"getStageCanvas",value:function(){return this.stageCanvas}},{key:"getMouseLayer",value:function(){return this.mouseLayer}}])})(Ar),Ws=ig;var fu=it(Ks(),1),gu=it(tl(),1),pu=it(ll(),1);var ri={};Ti(ri,{decode:()=>On,encode:()=>Et,format:()=>mn,parse:()=>Hn});var ul={};function Rg(e){let n=ul[e];if(n)return n;n=ul[e]=[];for(let t=0;t<128;t++){let r=String.fromCharCode(t);n.push(r)}for(let t=0;t<e.length;t++){let r=e.charCodeAt(t);n[r]="%"+("0"+r.toString(16).toUpperCase()).slice(-2)}return n}function Yr(e,n){typeof n!="string"&&(n=Yr.defaultChars);let t=Rg(n);return e.replace(/(%[a-f0-9]{2})+/gi,function(r){let a="";for(let i=0,o=r.length;i<o;i+=3){let s=parseInt(r.slice(i+1,i+3),16);if(s<128){a+=t[s];continue}if((s&224)===192&&i+3<o){let l=parseInt(r.slice(i+4,i+6),16);if((l&192)===128){let u=s<<6&1984|l&63;u<128?a+="\uFFFD\uFFFD":a+=String.fromCharCode(u),i+=3;continue}}if((s&240)===224&&i+6<o){let l=parseInt(r.slice(i+4,i+6),16),u=parseInt(r.slice(i+7,i+9),16);if((l&192)===128&&(u&192)===128){let d=s<<12&61440|l<<6&4032|u&63;d<2048||d>=55296&&d<=57343?a+="\uFFFD\uFFFD\uFFFD":a+=String.fromCharCode(d),i+=6;continue}}if((s&248)===240&&i+9<o){let l=parseInt(r.slice(i+4,i+6),16),u=parseInt(r.slice(i+7,i+9),16),d=parseInt(r.slice(i+10,i+12),16);if((l&192)===128&&(u&192)===128&&(d&192)===128){let c=s<<18&1835008|l<<12&258048|u<<6&4032|d&63;c<65536||c>1114111?a+="\uFFFD\uFFFD\uFFFD\uFFFD":(c-=65536,a+=String.fromCharCode(55296+(c>>10),56320+(c&1023))),i+=9;continue}}a+="\uFFFD"}return a})}Yr.defaultChars=";/?:@&=+$,#";Yr.componentChars="";var On=Yr;var cl={};function Ag(e){let n=cl[e];if(n)return n;n=cl[e]=[];for(let t=0;t<128;t++){let r=String.fromCharCode(t);/^[0-9a-z]$/i.test(r)?n.push(r):n.push("%"+("0"+t.toString(16).toUpperCase()).slice(-2))}for(let t=0;t<e.length;t++)n[e.charCodeAt(t)]=e[t];return n}function Jr(e,n,t){typeof n!="string"&&(t=n,n=Jr.defaultChars),typeof t>"u"&&(t=!0);let r=Ag(n),a="";for(let i=0,o=e.length;i<o;i++){let s=e.charCodeAt(i);if(t&&s===37&&i+2<o&&/^[0-9a-f]{2}$/i.test(e.slice(i+1,i+3))){a+=e.slice(i,i+3),i+=2;continue}if(s<128){a+=r[s];continue}if(s>=55296&&s<=57343){if(s>=55296&&s<=56319&&i+1<o){let l=e.charCodeAt(i+1);if(l>=56320&&l<=57343){a+=encodeURIComponent(e[i]+e[i+1]),i++;continue}}a+="%EF%BF%BD";continue}a+=encodeURIComponent(e[i])}return a}Jr.defaultChars=";/?:@&=+$,-_.!~*'()#";Jr.componentChars="-_.!~*'()";var Et=Jr;function mn(e){let n="";return n+=e.protocol||"",n+=e.slashes?"//":"",n+=e.auth?e.auth+"@":"",e.hostname&&e.hostname.indexOf(":")!==-1?n+="["+e.hostname+"]":n+=e.hostname||"",n+=e.port?":"+e.port:"",n+=e.pathname||"",n+=e.search||"",n+=e.hash||"",n}function Kr(){this.protocol=null,this.slashes=null,this.auth=null,this.port=null,this.hostname=null,this.hash=null,this.search=null,this.pathname=null}var Dg=/^([a-z0-9.+-]+:)/i,wg=/:[0-9]*$/,kg=/^(\/\/?(?!\/)[^\?\s]*)(\?[^\s]*)?$/,Cg=["<",">",'"',"`"," ","\r",`
`,"	"],Fg=["{","}","|","\\","^","`"].concat(Cg),Lg=["'"].concat(Fg),dl=["%","/","?",";","#"].concat(Lg),hl=["/","?","#"],Pg=255,fl=/^[+a-z0-9A-Z_-]{0,63}$/,Ig=/^([+a-z0-9A-Z_-]{0,63})(.*)$/,gl={javascript:!0,"javascript:":!0},pl={http:!0,https:!0,ftp:!0,gopher:!0,file:!0,"http:":!0,"https:":!0,"ftp:":!0,"gopher:":!0,"file:":!0};function Gg(e,n){if(e&&e instanceof Kr)return e;let t=new Kr;return t.parse(e,n),t}Kr.prototype.parse=function(e,n){let t,r,a,i=e;if(i=i.trim(),!n&&e.split("#").length===1){let u=kg.exec(i);if(u)return this.pathname=u[1],u[2]&&(this.search=u[2]),this}let o=Dg.exec(i);if(o&&(o=o[0],t=o.toLowerCase(),this.protocol=o,i=i.substr(o.length)),(n||o||i.match(/^\/\/[^@\/]+@[^@\/]+/))&&(a=i.substr(0,2)==="//",a&&!(o&&gl[o])&&(i=i.substr(2),this.slashes=!0)),!gl[o]&&(a||o&&!pl[o])){let u=-1;for(let f=0;f<hl.length;f++)r=i.indexOf(hl[f]),r!==-1&&(u===-1||r<u)&&(u=r);let d,c;u===-1?c=i.lastIndexOf("@"):c=i.lastIndexOf("@",u),c!==-1&&(d=i.slice(0,c),i=i.slice(c+1),this.auth=d),u=-1;for(let f=0;f<dl.length;f++)r=i.indexOf(dl[f]),r!==-1&&(u===-1||r<u)&&(u=r);u===-1&&(u=i.length),i[u-1]===":"&&u--;let g=i.slice(0,u);i=i.slice(u),this.parseHost(g),this.hostname=this.hostname||"";let h=this.hostname[0]==="["&&this.hostname[this.hostname.length-1]==="]";if(!h){let f=this.hostname.split(/\./);for(let y=0,v=f.length;y<v;y++){let p=f[y];if(p&&!p.match(fl)){let m="";for(let b=0,x=p.length;b<x;b++)p.charCodeAt(b)>127?m+="x":m+=p[b];if(!m.match(fl)){let b=f.slice(0,y),x=f.slice(y+1),_=p.match(Ig);_&&(b.push(_[1]),x.unshift(_[2])),x.length&&(i=x.join(".")+i),this.hostname=b.join(".");break}}}}this.hostname.length>Pg&&(this.hostname=""),h&&(this.hostname=this.hostname.substr(1,this.hostname.length-2))}let s=i.indexOf("#");s!==-1&&(this.hash=i.substr(s),i=i.slice(0,s));let l=i.indexOf("?");return l!==-1&&(this.search=i.substr(l),i=i.slice(0,l)),i&&(this.pathname=i),pl[t]&&this.hostname&&!this.pathname&&(this.pathname=""),this};Kr.prototype.parseHost=function(e){let n=wg.exec(e);n&&(n=n[0],n!==":"&&(this.port=n.substr(1)),e=e.substr(0,e.length-n.length)),e&&(this.hostname=e)};var Hn=Gg;var li={};Ti(li,{Any:()=>ai,Cc:()=>ii,Cf:()=>zg,P:()=>jn,S:()=>oi,Z:()=>si});var ai=/[\0-\uD7FF\uE000-\uFFFF]|[\uD800-\uDBFF][\uDC00-\uDFFF]|[\uD800-\uDBFF](?![\uDC00-\uDFFF])|(?:[^\uD800-\uDBFF]|^)[\uDC00-\uDFFF]/,ii=/[\0-\x1F\x7F-\x9F]/,zg=/[\xAD\u0600-\u0605\u061C\u06DD\u070F\u0890\u0891\u08E2\u180E\u200B-\u200F\u202A-\u202E\u2060-\u2064\u2066-\u206F\uFEFF\uFFF9-\uFFFB]|\uD804[\uDCBD\uDCCD]|\uD80D[\uDC30-\uDC3F]|\uD82F[\uDCA0-\uDCA3]|\uD834[\uDD73-\uDD7A]|\uDB40[\uDC01\uDC20-\uDC7F]/,jn=/[!-#%-\*,-\/:;\?@\[-\]_\{\}\xA1\xA7\xAB\xB6\xB7\xBB\xBF\u037E\u0387\u055A-\u055F\u0589\u058A\u05BE\u05C0\u05C3\u05C6\u05F3\u05F4\u0609\u060A\u060C\u060D\u061B\u061D-\u061F\u066A-\u066D\u06D4\u0700-\u070D\u07F7-\u07F9\u0830-\u083E\u085E\u0964\u0965\u0970\u09FD\u0A76\u0AF0\u0C77\u0C84\u0DF4\u0E4F\u0E5A\u0E5B\u0F04-\u0F12\u0F14\u0F3A-\u0F3D\u0F85\u0FD0-\u0FD4\u0FD9\u0FDA\u104A-\u104F\u10FB\u1360-\u1368\u1400\u166E\u169B\u169C\u16EB-\u16ED\u1735\u1736\u17D4-\u17D6\u17D8-\u17DA\u1800-\u180A\u1944\u1945\u1A1E\u1A1F\u1AA0-\u1AA6\u1AA8-\u1AAD\u1B4E\u1B4F\u1B5A-\u1B60\u1B7D-\u1B7F\u1BFC-\u1BFF\u1C3B-\u1C3F\u1C7E\u1C7F\u1CC0-\u1CC7\u1CD3\u2010-\u2027\u2030-\u2043\u2045-\u2051\u2053-\u205E\u207D\u207E\u208D\u208E\u2308-\u230B\u2329\u232A\u2768-\u2775\u27C5\u27C6\u27E6-\u27EF\u2983-\u2998\u29D8-\u29DB\u29FC\u29FD\u2CF9-\u2CFC\u2CFE\u2CFF\u2D70\u2E00-\u2E2E\u2E30-\u2E4F\u2E52-\u2E5D\u3001-\u3003\u3008-\u3011\u3014-\u301F\u3030\u303D\u30A0\u30FB\uA4FE\uA4FF\uA60D-\uA60F\uA673\uA67E\uA6F2-\uA6F7\uA874-\uA877\uA8CE\uA8CF\uA8F8-\uA8FA\uA8FC\uA92E\uA92F\uA95F\uA9C1-\uA9CD\uA9DE\uA9DF\uAA5C-\uAA5F\uAADE\uAADF\uAAF0\uAAF1\uABEB\uFD3E\uFD3F\uFE10-\uFE19\uFE30-\uFE52\uFE54-\uFE61\uFE63\uFE68\uFE6A\uFE6B\uFF01-\uFF03\uFF05-\uFF0A\uFF0C-\uFF0F\uFF1A\uFF1B\uFF1F\uFF20\uFF3B-\uFF3D\uFF3F\uFF5B\uFF5D\uFF5F-\uFF65]|\uD800[\uDD00-\uDD02\uDF9F\uDFD0]|\uD801\uDD6F|\uD802[\uDC57\uDD1F\uDD3F\uDE50-\uDE58\uDE7F\uDEF0-\uDEF6\uDF39-\uDF3F\uDF99-\uDF9C]|\uD803[\uDD6E\uDEAD\uDED0\uDF55-\uDF59\uDF86-\uDF89]|\uD804[\uDC47-\uDC4D\uDCBB\uDCBC\uDCBE-\uDCC1\uDD40-\uDD43\uDD74\uDD75\uDDC5-\uDDC8\uDDCD\uDDDB\uDDDD-\uDDDF\uDE38-\uDE3D\uDEA9\uDFD4\uDFD5\uDFD7\uDFD8]|\uD805[\uDC4B-\uDC4F\uDC5A\uDC5B\uDC5D\uDCC6\uDDC1-\uDDD7\uDE41-\uDE43\uDE60-\uDE6C\uDEB9\uDF3C-\uDF3E]|\uD806[\uDC3B\uDD44-\uDD46\uDDE2\uDE3F-\uDE46\uDE9A-\uDE9C\uDE9E-\uDEA2\uDF00-\uDF09\uDFE1]|\uD807[\uDC41-\uDC45\uDC70\uDC71\uDEF7\uDEF8\uDF43-\uDF4F\uDFFF]|\uD809[\uDC70-\uDC74]|\uD80B[\uDFF1\uDFF2]|\uD81A[\uDE6E\uDE6F\uDEF5\uDF37-\uDF3B\uDF44]|\uD81B[\uDD6D-\uDD6F\uDE97-\uDE9A\uDFE2]|\uD82F\uDC9F|\uD836[\uDE87-\uDE8B]|\uD839\uDDFF|\uD83A[\uDD5E\uDD5F]/,oi=/[\$\+<->\^`\|~\xA2-\xA6\xA8\xA9\xAC\xAE-\xB1\xB4\xB8\xD7\xF7\u02C2-\u02C5\u02D2-\u02DF\u02E5-\u02EB\u02ED\u02EF-\u02FF\u0375\u0384\u0385\u03F6\u0482\u058D-\u058F\u0606-\u0608\u060B\u060E\u060F\u06DE\u06E9\u06FD\u06FE\u07F6\u07FE\u07FF\u0888\u09F2\u09F3\u09FA\u09FB\u0AF1\u0B70\u0BF3-\u0BFA\u0C7F\u0D4F\u0D79\u0E3F\u0F01-\u0F03\u0F13\u0F15-\u0F17\u0F1A-\u0F1F\u0F34\u0F36\u0F38\u0FBE-\u0FC5\u0FC7-\u0FCC\u0FCE\u0FCF\u0FD5-\u0FD8\u109E\u109F\u1390-\u1399\u166D\u17DB\u1940\u19DE-\u19FF\u1B61-\u1B6A\u1B74-\u1B7C\u1FBD\u1FBF-\u1FC1\u1FCD-\u1FCF\u1FDD-\u1FDF\u1FED-\u1FEF\u1FFD\u1FFE\u2044\u2052\u207A-\u207C\u208A-\u208C\u20A0-\u20C1\u2100\u2101\u2103-\u2106\u2108\u2109\u2114\u2116-\u2118\u211E-\u2123\u2125\u2127\u2129\u212E\u213A\u213B\u2140-\u2144\u214A-\u214D\u214F\u218A\u218B\u2190-\u2307\u230C-\u2328\u232B-\u2429\u2440-\u244A\u249C-\u24E9\u2500-\u2767\u2794-\u27C4\u27C7-\u27E5\u27F0-\u2982\u2999-\u29D7\u29DC-\u29FB\u29FE-\u2B73\u2B76-\u2BFF\u2CE5-\u2CEA\u2E50\u2E51\u2E80-\u2E99\u2E9B-\u2EF3\u2F00-\u2FD5\u2FF0-\u2FFF\u3004\u3012\u3013\u3020\u3036\u3037\u303E\u303F\u309B\u309C\u3190\u3191\u3196-\u319F\u31C0-\u31E5\u31EF\u3200-\u321E\u322A-\u3247\u3250\u3260-\u327F\u328A-\u32B0\u32C0-\u33FF\u4DC0-\u4DFF\uA490-\uA4C6\uA700-\uA716\uA720\uA721\uA789\uA78A\uA828-\uA82B\uA836-\uA839\uAA77-\uAA79\uAB5B\uAB6A\uAB6B\uFB29\uFBB2-\uFBD2\uFD40-\uFD4F\uFD90\uFD91\uFDC8-\uFDCF\uFDFC-\uFDFF\uFE62\uFE64-\uFE66\uFE69\uFF04\uFF0B\uFF1C-\uFF1E\uFF3E\uFF40\uFF5C\uFF5E\uFFE0-\uFFE6\uFFE8-\uFFEE\uFFFC\uFFFD]|\uD800[\uDD37-\uDD3F\uDD79-\uDD89\uDD8C-\uDD8E\uDD90-\uDD9C\uDDA0\uDDD0-\uDDFC]|\uD802[\uDC77\uDC78\uDEC8]|\uD803[\uDD8E\uDD8F\uDED1-\uDED8]|\uD805\uDF3F|\uD807[\uDFD5-\uDFF1]|\uD81A[\uDF3C-\uDF3F\uDF45]|\uD82F\uDC9C|\uD833[\uDC00-\uDCEF\uDCFA-\uDCFC\uDD00-\uDEB3\uDEBA-\uDED0\uDEE0-\uDEF0\uDF50-\uDFC3]|\uD834[\uDC00-\uDCF5\uDD00-\uDD26\uDD29-\uDD64\uDD6A-\uDD6C\uDD83\uDD84\uDD8C-\uDDA9\uDDAE-\uDDEA\uDE00-\uDE41\uDE45\uDF00-\uDF56]|\uD835[\uDEC1\uDEDB\uDEFB\uDF15\uDF35\uDF4F\uDF6F\uDF89\uDFA9\uDFC3]|\uD836[\uDC00-\uDDFF\uDE37-\uDE3A\uDE6D-\uDE74\uDE76-\uDE83\uDE85\uDE86]|\uD838[\uDD4F\uDEFF]|\uD83B[\uDCAC\uDCB0\uDD2E\uDEF0\uDEF1]|\uD83C[\uDC00-\uDC2B\uDC30-\uDC93\uDCA0-\uDCAE\uDCB1-\uDCBF\uDCC1-\uDCCF\uDCD1-\uDCF5\uDD0D-\uDDAD\uDDE6-\uDE02\uDE10-\uDE3B\uDE40-\uDE48\uDE50\uDE51\uDE60-\uDE65\uDF00-\uDFFF]|\uD83D[\uDC00-\uDED8\uDEDC-\uDEEC\uDEF0-\uDEFC\uDF00-\uDFD9\uDFE0-\uDFEB\uDFF0]|\uD83E[\uDC00-\uDC0B\uDC10-\uDC47\uDC50-\uDC59\uDC60-\uDC87\uDC90-\uDCAD\uDCB0-\uDCBB\uDCC0\uDCC1\uDCD0-\uDCD8\uDD00-\uDE57\uDE60-\uDE6D\uDE70-\uDE7C\uDE80-\uDE8A\uDE8E-\uDEC6\uDEC8\uDECD-\uDEDC\uDEDF-\uDEEA\uDEEF-\uDEF8\uDF00-\uDF92\uDF94-\uDFEF\uDFFA]/,si=/[ \xA0\u1680\u2000-\u200A\u2028\u2029\u202F\u205F\u3000]/;var Bg=[8364,0,8218,402,8222,8230,8224,8225,710,8240,352,8249,338,0,381,0,0,8216,8217,8220,8221,8226,8211,8212,732,8482,353,8250,339,0,382,376];function Ng(e){return e===0||e>=55296&&e<=57343||e>1114111}function vl(e){return Ng(e)?65533:e>=128&&e<=159&&Bg[e-128]||e}function ml(e){return e-1>>>0<127||e-160>>>0<55136?String.fromCharCode(e):String.fromCodePoint(vl(e))}var Je=(()=>{let e=new Uint8Array(127),n=0;for(let t=33;t<=126;t++)t!==34&&t!==36&&t!==92&&(e[t]=n++);return e})();function bl(e,n,t,r,a,i){let s=e.length,l=i*90,u=0,d=()=>{let S=Je[e.charCodeAt(u++)];return S<i?S:S*91-l+Je[e.charCodeAt(u++)]},c=t-r,g=t+a,h=new Int32Array(g);h.fill(-1,r,i),h.fill(-1,i+c,g);let f=new Int32Array(g),y=new Int32Array(g);function v(S,k){let L=0,C=k,z=k+S;for(;C<z;){let F=Je[e.charCodeAt(u++)];if(F<89)L+=F,h[C++]=L;else if(F===89){let P=Je[e.charCodeAt(u++)]+2;for(;P--;)h[C++]=++L}else{let P=Je[e.charCodeAt(u++)];L+=89+(P<90?P*91+Je[e.charCodeAt(u++)]:Je[e.charCodeAt(u++)]*8281+Je[e.charCodeAt(u++)]*91+Je[e.charCodeAt(u++)]),h[C++]=L}}}v(r,0),v(c,i);let p=new Int32Array(a*2),m=0,b=0;function x(S,k){for(let L=0;L<S;L++){let C=k+L,z=d(),F=d();p[b*2]=z,p[b*2+1]=F,b+=1,f[C]=m;let P=(h[z]<0?y[z]:1)+(h[F]<0?y[F]:1);y[C]=P,m+=P}}x(a-i+r,i+c),x(i-r,r);let _=new Uint16Array(m),T=0;for(let S=0;S<b;S++)for(let k=0;k<2;k++){let L=p[S*2+k],C=h[L];if(C<0){let z=f[L],F=z+y[L];for(;z<F;)_[T++]=_[z++]}else _[T++]=C}let R=new Uint16Array(n),A=0;for(;u<s;){let S=Je[e.charCodeAt(u++)];S>=i&&(S=S*91-l+Je[e.charCodeAt(u++)]);let k=h[S];if(k<0){let L=f[S],C=L+y[S];for(;L<C;)R[A++]=_[L++]}else R[A++]=k}return R}var yl=bl("!}.&u%}'&}*'~!6*)%&,~!J~!J~%L~y<~!R,~~%Lu~~#GD~~#|)1#%}^%}2%+#.##%##%}&%##%'#%##&%#%#'%#&#%#&#'#%%#&#%##%#)%''%&%#%#'%#%%#%%}%%%#%#&(23#%%#&-%0%('1#(##%#'##+%'*.:1}#%#6-+(%'%%#%%%}#L'2351&('%}&/N'(0(/*-%(%%}#'+&T%7.2}#&%&#%#36/5##%&%%#&#%%#))2%%##%&&'0~!#*+&'%1~!%).'3q?&%'1~!.##%6(~!+%%%(Gw'rT~!E#<nA%#jZ~!H%(~!42##~!*31&~!G%U~#)5~#`3~!J~!Z~%]~%Y~%C~!q~!u~#kz~%#~!6'~!D~!U~!?~#T~!c%~!G#'~%7|~!G~!J~!G&~#pb~(Df}#%}*&}#%##%##%##&#-}&'#'&%#.++}%mI,#,@&(}*%}*'%&##&#%##%}&0}#.},U},%}+%}&%}#%##&}B%(}(%}+%)})%##%#&}&%##%&}<%}>%#%&}*%}(%}9%}/%})%}*%}*%}?&}&%}3%}&*#%})%#%#)}#&#-#+*%E%%'%'#%}#*V##&##I}#&&##%&%#&&Qf%%))w/0+&%#(#.%-''''++++7}>%4'',##1,#%#&%##&#'##&#*#9)%&%}#*}%,#+P(%A&%#'&##wSD',9E00#y#@}(+}&%&>~!#~!X}#*}(&&}(&}(,%}%&#+&}#&}I%#%}%)#(},'%#*}4%%#%}(''}#/##(##),%-##%%)#&}(.}&%#&}%%}*&#%},&&}&%}#%*'#%})%}D&}&%}-&}6&#&}-,%}#%})-(~+`~,=?~I9'9%~!,#%})%})%}@%}?%}(~!?~#<~#pP~#BG~#=1#%K+~#?#~%;)~#A~#mF1~#A'~'X%'~#lR~#N~'N~#r~#m#-~#i'?%#'%~#B%##%,%#~#_%#0%~#]732~,w~2+#:&#%&'0%&>%}#>##F+)#%&&#(+_}4&}-%}(&}@&}O7Fdf0@+/v4}&WU##&/0#&'('B#%}.%}'+#%}#%%&#&%#%##+#&#)#6#'#.},%}c%},%#%##%&#&%#&~#>'*-.%##%##%}#%%}%'~#)D1}#%*&~#_%%'(~#S2%'.}#~#=##*'*-%}&'%'##&&~'E%.#&~#M4}%%##&'%#~#O1##%&#'+~#<B%##%%'%+~#;#@%}#&%#&&%#(~#H1}'%'##&&~#?A}&'~#D#%32}'&&&&~#[}'(#%}'~#;C})&}%%#%~#=&%,3}%'(#%%~#^'#&&)#%'~#Y%-~#d-%'~#^%%&#&&&}#~#b~2t*&'~&(~&@~0%~e~3}%*''0})&}+~!9##-}#%-hD*)1fC#%/&/fB#40~!+#)*4~!+~!K'&:~!/*7~!.#~!H~!L':~%x&~!H#~!*~%1~!I#~!+A~#p'~!F~~#-#~,,(~.Z~!V~%;'B'mq-W~!N~%I%#&&#&}#%},%%}'%}+X#%}#&}(%}'%}<%}#%}%%'}'%}:~![)9@~%>~#UA%-%##&~!C%~!-.9:~!1~!-^2/:a~!y,D*J#-5)/4~%23,~#G~!L1~!0X3`~!2+~!!0-~&E~!W~!o,>Y&]~%cZx_&~#O*9#A#'#+I'%#)~!0B*-5A+-((F&*M#)(-7-5+'-3a5Vi~!Y~!?+[)%3),ERHm~!+:D,VG.+)?fB%%*(%)'(#&80%1'8`K8?`+'Z#&O&'H5#*9)A%%5&3))0%39+.*7#()&&*=4@**L)<'_&*+..;(#*+)./&0#3)%')-8(4ixD(&.}%,('aI:,)%,k2231T)I'#/-W7,/'Q#.'Y24+h')37</31&83##&0#),H(?'&?/1##%#&&#%''-%&&&#(&''&#.-'%#%%(,')*'&#&#'##%(%(#%('#&##%%%%('%#%#%%#%#&%##h>w+v<ayvyvcg.uuhKr}g/v|g>u9i[~>g5uI~=RvdwEg;v/g;uk!!TTSx]@RT!U!#!@VBRUU!'UTe-d0c`e&gSdicedFcrdTaqb.kYcAohdYd@a3e+d}dMdtd.aJ#bqcK`dle/e.e'dwdPdodddjbEb}ogd^ofdpduc6j?l%d{drdqc)d7bacOdQ%T#Y)X.sR[yH>6Vyv3[xwLu>vo'!*.[yBacahoj>6Rew3[xqdZa#!a&#^(X-[yG>6Vyu3[xvg3sEr|g.u/Ri9db0T#^(Xa)!-[y;>6Vylg4wKs{JwNZt3@3r=c4Z([xlg;wKt!cpq's@v7A'*a(a+!-a#[y<3Dt?3Dt'>6Vym3[xmg9rxsNJwLZt4~?r?db1T#`-!(Xa,!0[yS>6Vz%NuQs.g4wKtnJwNZtS@3r>c4Z([y%g;wKtrdga8!a(!#&T*Y-Xa#!a0<or[yc3Dtq>6Vz43[y3JwNZtf@3s!Ju}!%Dti:pm3c_%X#tjB5pkd6q!r]u?voC'*-a.a2!0a&a+[yI3DtI3Ds~3DtH>6Vyw3[xx;:s#~<5pKJwNZtE@3r~d`a)!a2T#a.(!+U.X1[yT3Dt`3Dtv>6Vz&3[y&g9rxwzcxstPu.<rAJwLZtT~?r@dZa%!a.&^*Za(/Reu[ya>6Vz23[y1g3sEr}wkg{NuQRg{ci(U#5@b`~,cg#U(2WnH5wugcRh7dX#T(Y,a'Ta!!a,[yZ<]mj>6Vz,3[y+Pv#5ReZKu+=,%!H}7ABwkaS?Rh:BcW(X#<]mrj:ubv/ARekdg%!(!a.*Ta(Y.X1!#sP>Rl*Dt6[y>>6Vyo3Wf*jOvuumvuRgRJuq*!:9<B@bX~3jVv&v@s@5Re[d/rQt{uAvo&a&a*)a2!,0Wf!3Dt0=Bs'>6Re}3[xy~<5s%JwJZt1~Gs)c;&!#2sJkNuXvzq7rxu,Re8dka4!a8(aEZ+a@Y.X1Xa)[yd=Bs(3DtP>6Vz53[y4cX#X&Re:avRe9~<5s&JwJZtQ~Gs*i^rzvdRg+Jv{%!2sbB@bX}kdga,!Za?&^*T1/!a'Dt+[y6>6Vyf3Wf%g/u;s4hGu6?Rh-JvZ,!c%#&RoX54Rivj7uyvf8RgTKvZB%*!2sGh<vu5Rgq<=C::9bb~#dZ#T&Ta6Y.X*Dt>[y93Wf)coZ(T,6VyifluvRgC@95@B@bX~/hFu34cC#T,k/unq8w8Q5RkUklwQuzunq8w8Q5Rk8d/rJu?v8w9)-&!a0a;a&aIWejg3sEr/h1s<DtDJvyZqY5aws3Jvy!&Wei~Hr1:au5@Bag>23E~5c:Z&bX};kKv?w&unuVu5Rjc;>bs)#~@:Rh.=ay<a]C;b`}Vd6s/t{uAvoaxa()!a,a7%-a#a2Dt,[yF2Wo[>6Vyt3[xuNuPRi&NuPwpi#RoWh?vf8Ri%Jv]!%Ri:KvxD!.'2WeAjZu`q9rxu,Re7woeAg-unLq(qA_/*2Wg_g3u5q^9:4E}/jTrxrzv=Wkkd~0UX#^^Xa-a1a5T&a=U1a'*aEa]!a*aPaA-adok[y54Rn>;:p3~Dp5g9rpsFNvZqjg3uJp4~<5p0Pw;5qlJwNZt*@3p1Pw:5p/Ou!5p2JvG'!6Vye=<qnJvh_[xhg3v,Rh3kOwOw-sDuev/Re^dha[a%!%!a+#Ta7)-5TaCaO!aka!a)sf[yb2>Rl!9ARiq5E}Qg=ucRkBE|oJrJ_@Wk~@Wk{JrJ_@Wk|@WkyJrJ_@Wk}@WkzJvO_[y2g-vMRmiKuYC!)&>Ri;>Ri<@3RkNc](X#@9Rk=g5vuRmhKvDB!+'=]meg3u4Rmgd)#Y'Vz3CARmfd`a+!%T'!+#Ta1Ta6TaM-sTDt9[yA9sYd'%Y#s[[xpj:ueunaXRgEjRq,v-vuqdd2'`#6Rev<32@5>:2<E}5xIo9a*X#Y(;5RePJvD_g>vyRgNj8w)v8<wggs:RgXiZt|vjx,hSq3ah!-(~@:Ro/Ou!5RhWj^v(pyw8unRhUdx-UY#^Ua.a3a70!)%UX1TaDa)'omRiRRhE[y:3Dsz=Br,>6Vyj3[xkg6ruwjcqsrPw;5r*Ku]D'Zt-@3r(~?r.i[vwv]dU1a--U#`a4(g/vsRhPOu!5RhLj:rmu9Wo!~@:wdh@g/vsRiTjXuvvNr}:RhBj^v(pyw8unRn]dz1UYa'a+^Y(!aETZalaRY.Ta?a4[yDJw1!#qLsW>6Vyrfzq-pLflpwRe|Js>%!Dt@3Dt&Jvy_[xs~HrnjMuwpsw'RecKu+D#'!t<~Grl~?rjg5u-x,gwp{ah!-(~@:Rg~Ou!5Rh'jXuvvNr}:Rh#cW#X/c;&!#2sLi[v7u7RgpJv)(!iLrxu,Re6j7v@s@5Se[e7d`aW!Za(a`T.a#!a3!&aDa-!9)Dt_=6s+3[x~~DR|h~DS6avhGun5RkZj3w)v-]mkKunB!&*]kb97R|i<ARk<c:Z(6Vy}Juh'!wziMRoS:F|vkLuauJv5vtvQRh1d='T+Y#VyO~DR|jcF#T'7R|g97R|kJv3'!ay<Rj,Jvh&!:ReXcsa6*a+#a#_aIRf9aLRf?c,Z&Rf5Rf7c.Z&Rf;Rf>cQ#%T'p-Rf8Rf=ct#%'(*!,p,Rf4p+Rf6Rf:Rf<d~'Ua%U*^UYa(!a,-!#a4YaTalaEX0a8a<Weo3Dt/3Dsx=Br93Wen~Dr;~<5p<JwNZt2@3p=Pw:5p;Ou!5r3c7&!#:p>3Ds}KvGB)_6Vyk2sM=<r7x'eovA(!hFu1ARf}cV#X&@r5j6rvwQa^Rf3c=Za'wkghJv__g;unRggA53B9=b^}%j6uduo5Jq;!(hIv%2Re`Ou4ARe_e%a#^^^Xa&!a*a2!&a6YaP!*ad!#a:aE/5Rn?[y@>6Vyp;:pE~DrY~<5pBJwNZt8@3pCh=rt3rWPw:5pAJup_[xoNuPpF9c!#'45pD5ARn)d8#X'X*3@rU72s]h>v<<sSjJpqvewOJq/(!hNw'5ReBk0s2u3w/w'5ReE5@Jq.!a+JQ!&WeU23d(#Y&RjG5]jBk!u7w&u0udARjEe#+^^^Ub#!a2/a`Z(agT1!a-a;|@TaG!aS[yV=Re~fow'RguNuPRe?bz#'>RoUWeL>:Cbb|?JwPZtVg6ruRmzJvD'!6Vz(g/vmRh~Jvy_[y(g9voRgyx*cy(#2>Ri2B9b]~9kIw9u7rluJu3Rg]dI#a%UY'@=p%CAx.gQZ&RhwwygtRm{x5g_Z'+ABqR9Woa=Bp&dV#^*Xa'!&@o{g4v]Rk;Jv{!%Rk[wkkiA5RkiwwfUB=x,fUuqC&*!>RfTg8v0RfV~ARfSd;rJsAuAv9wR'ae+/aO!a@aza/a#[yQ@Wg!2Wemg3sEr0JvB_g>uvReWg2v+Re=KupB_+[y!2AbY~-~Hr2AJwD!(h<~El>h<~El?Kun@+_:9b`}Kg-v/Ri3g;vtwyk_9]k_d=&T#*U.6qh@Ab`|K9:H|CJv[!&3Dtex'fDwC%!Rf[9WlMd[(^X,!a%Z06Vz!@WgBg=v~Rgvg,QRe@awd,#Y+jTv|Q~EfWj]uNr|~FRfXdy#Y&^Ua%!aO.!(a)Ua;=!a@aKap!a-,a!Ta]a[rSa]p?[y82sK=Bq~;:p:~<5p8Pw:5p7d'#Y'Wf(;RnRi[u4w&RgJJvG'!6Vyh=<r#ijuuv/sIKuYD'ZtG@3p9~Gr&d2#`(g<vtRgFj`u5w&rqpxRf2CJuY!+:wfnTOu!5Rg}jNs1ucv&RfwJvA!&3@q|BDcC#T,k/unq8w8Q5RkTklwQuzunq8w8Q5Rk9dga#!a'!a=#a0!:+Tb*b@aO.a4!aba8aFJv^}?!VyR~Dr<g;u%Rn.~<5p[x'e`wNZtR@3p]Pw:5pZhNvjBp.woe_g5u-r4JwF!%DtO3:ooc7&!#:p^3DtpLuGw(!+%)Dtk6Vz#2sd=<r8d'#Y([y#<x3gJt`w@!)%}MRiowzikRij=]ilxAf3,U(#B2Rf#g0v-Rm[ck{`U#]giKv3>)!&6Ri154s,KuGB_%@r68r:dJ|t`#X(9<E|u2@H|rx3gJu?w'!+'1Nu7Reg4=H~+9<wxgY95Rm]xLggZ-`(X}U2:Ri4h<uOawRmsJv__5@bb{jbV~3dka#a'a]!,#a+U=a>b6a3b%!/aKa/)!arwve^VyJ;:pR~DpTg3uJpS~<5pOPw;5qmPw:5pNOu!5pQJvG'!6Vyx=<qoJvA!{~Jup!%@qk7Rn/KvyD!}''[xz;>wkh'?Rh,x8gyt`w5D!&),(SgyccRgztJ@3pPB5p#d'(Y#<]mmifubw&RgoJvE&!82s^JvF&!8Rf,ADb]~;x=h'rNu]vK!,%'*0RnORh)4Rh*AqQg-vaRnNg;wHwkh'ba~4cE#Ta*x3gctyw@'!+%RnFRnD<4Rn@hFvK5RnCxWg[#`&a0Ua()`1Rm75Rg[c]%X#qi8Rg^NvdRj>BwzgZauwji7Rm6A4wgg]d1#&(*,.0a#Rm;Rm<Rm=Rm>Rm?Rm@RmARmBe%#^^^Xaea?aC/b+(,!a+a#!a/!>a&Ta<aKbD!2wphBRnk[yPw}hE|.=Br-3Dtm>6Vy~g6urRf.x,hPrNav!%'RnqRo%Ro#Nu;q[Pw;5r+JwNZtM@3r)d'#Y'Weh;xChL#`&RnmRnoKu}>%(!Rne~Bs-;2wjcussJv+'!aYSO}6@B<5?ba~8LrNvj!.%*ROwungw~ng~:9;Ri^>wtnig;wHRnixDh@|(UZ.x1h@|)!#:2<H|*xHn]#-UX'3Ro)z=iT}6ARns=Bwsn_wpnaRncw]aR(#UXa&Ua*a/=]iPd'#Y&Ro'WnXf{QRm2hNvj]nZd`'T~&1`{|`#9b]{}c:'!#Wl{>@=be}]?cl{{U#:5Abb}Jds#^YaF!a*b4a#a3aPa>&Tb!bH!*a_!Eau?/a&RjY<]gj>6Vz*;:pe~DrZg,QRj1JwNZtX@wihspcJvZ&!VyX9WmOJu|!|N2WmHJvh&!]ht~Bpbcn&T(!#RmQ<s7Nu;padH#X'`+WmJ@>RmKCARhnKup=!)&Wf+:RhqNuPpf9c!#'45pd5AwghpARn(Ls@w!%,)!RmP@Wfe<E|IJva!&WmNg8vsRmLd`*.`#Y'Xa!axRn*]hrA8Rhug5s@rXg8u!RmMd8#X'X*3@rV72smdI*#UY&RmICARho~GsgxVgd)Ta'U-Y&Xa!T#RnEWnA@Wffg1uDRi0hFvK5RnBxGnG&#`%owp)@wsf+bX}Ze-*1!a*^^^Ua|!#a.aq&Ya2!a>.a6!a:aO`aJDtL[y`@Wg#>6Vz12@wzoYRoZNuPRi!NuPRhzg=ucRi,@=b`{Yg=ucRi-ACJvB!&Sh[ebSh]ebi`wUuFRm4Jw2_[y0JvB!.<Ju(!&SoG}6Shd}6<Ju(!&SoH}6She}6Kur@._g5vHRieJvx!{L2G{Kx6gd'T#?Rh82Wi5cZ#X(g1w)Rm5dW-Y(Ta#!a)!#aYa=wnfE=su2>>bU{0j9udv:<svj8uQv-7RgHdE%#^'sq9sp=>Bb_{TJv`!&g/r|snj6v(us5d,#Y(56H}[978H}]Jw5!&g1rushJvB!+j;v{u5?zDhd}6}bj;v{u5?zDhe}6}ce*#`(^^^a[aea!=!a6a*aoXb1a.!aAbL!b>,b'aL!aV@Wf|2Wlg3[y/JwNZt^@3piPw:5pgJunZou3@rsJva&!Vy_g<v~Rm#JvG'!6Vz0=<r{Ju{%!:pj@WfsiXuJu3Rm:JvZ&!WfA~Bph@c4Z&Dtwax5rubx(#:awRk1@d,#Y&RfjRfid1#,Y(@Wfp2Wlrg5s@ryKu[@!,'=]ig9wlk?Rk>g5u-rqJvy'!@9RkQcH(T#=>Ri~@<wkj(Wj(KuZB*!&<7rw@9RkRcH(T#=>Ri}@<wkj)Wj)dg(Ta2Xa9X#`-!a*CARhg@@=I}d9x;c~#X%so=<sj>2@@=aybb}XjWv0Q~EfEj3vLv;<d,#Y(56H}`978H}_dgaPaFa'a/!#a3Y0a_a;a|!1(a7-[yE3[xt;:pJNvZrrg3uJrvJwNZt=@3pIh=rt3rxPw:5pGOu!5rpJvG'!6Vys=<rz@c4Z&Dt(ax5rtJvZ!&~BpH@wsfNg-vaRlNci*U#=<wei<F}a5@Jq.!a*JQ!%@qZ23d(#Y&RjH5]jCk!u7w&u0udARjFd/prq=tyvpaEa(a:.!a1aZ(@@=I}:9wpd%=<sX55w_h}@@=I{t=ay<aU@@=I}T=ay<2@@=I})?C9:9au@9Cb]}DP~=x-fAZ(2Wl1=ay<aU@@=I}>5@d##Y+jTv|vV~EfFj]uNpn~FRfGdgaK!Z2&!a8a-Tb({E!acTbM*!a(DtY[yYd'%Y#sl[y*hHvh>Re5x2c{Z}.j4uCvcawRiMd+#X+_x&d!},<5RkX;2Hzw@x,gavfB-!{CcF&T#Roe;RodwWbBg5urRgaKvHC*_6Vz+<4opieuew&Rmq@d]&Y)X,T#X0Rh}<BqP=4qS9:ReMg/ujReNJw0!/<Jui%!bd{kawwnemRelAxUa?a3#*.&UX(Ya+a/RhvRnQ<o}9Wmtd-#Y&RgSRmw9;Rmxay=Rmyg-vaRmuxEhSrNu,v-voC!%(aR.a(a7+1Ro1>Ro5CE{A9b]{@;5x#eO{:g;urRi+KrNA!%(Ro3>Ro79;Ri_Ku@>{;&!x%gX|{KunA_+g5QRj/g3u5Rj#g>uERj%wio/xRhS&!,!#^1U}wba{8>>@=be}qC@:D5ba{7Ku+A&!}x?ba}t>>@=be}se(aA^^^Uat!b0#{pa+awUazbGa#aLb9bgaWac'a5TbS=Br!d1#`%scp_Jvl!#rT>Re0JvX&!VyN=H{Fcm#U&:pY=ReaJv2&!]h0=]nUJvG'!6Vy|=<r%JrM_=]h2@Wlud'#)U'Wf'b]{i=]h/Jvh!&~BpWg=v]RnMx+ny#'Nu;pVwjnu=]nwxJnx,T#`&Reqwjnt=]nvieu9vrRjLLuYwP(#+!th@wih5pX~Gr'g5v/Rh4KunA'!-CARnP@wwiN:Rm_9x'cvw>!|l=<saKvAA!0&3@q}>w^e1bp#&Re2Re3BDx7gH#T|f5H|eKuZ>!%(:qNAH{]Jv6!+3B2B9=b^{X<5<B92:E{ZLvhwA(a;a%!igQuyRmad+#Y}m@3Rh5d8#X'X*:AqUAHzmaxwbh<aXRnVcF}RT#Nw&cj#U(BWnug/vsRntdka)(a3+.Zb7aYYan1!bVa@Xa}[y^@b[{G=H{+hFu73Rj&Pv#5ReQcK%T#sig1v{Rj'Ku+D#'!t]~Grm~?rkKuMB!01d5#`'Vy.ta3Dtu~Hroc8#'{^45s85AwZbP&!#Rn!wghxWn#KvEA!)&2RlA2RlBx:h|#(T,=]j09Wobz>x]z/@awRoTd+#Y(az]hFhCrm4d,#Y+jTv|Q~EfMj]uNr|~FRfOdCa!Xa9_X#@<plJvf!%b`{(9;Rgwc;.!#2x7cw#T|UDb]|T5Ju={(!=@E{&Jv)&!Ab`{'awJvf!~*>>@=be{#KuY>!+&4Ezyi[ugv&RjIdea+T)#UXa&T-T&a!Rh9auRmW=]kLg5vuRn+g3u4Rn-Ow6ARn,hHus5xNk?#UX(U~)/g8v0RkD~AwkkF?Ri.OuNBwkkA?Ri/d|a2`a*^UYa.!aBTZaTa'Xa;!(!2!-a#b2[yC>6Vyq3[xr2Wi?g1rusVh%s?DtF~<5rbJs;%!DtBfswKtCj[uvuSsEu3RgVx3o:u+wN'*Zt;@3rd~Grh~?rfg8w)Lq)qE&-a%!>bI|`jWv0vV~EfCjTv|vV~Ef@j]uNpn~FRfBcK#T']gWNu7x,k7q4ai(0!hHv8<RhmkMu9vrsBuev/RhlCJvB!,g<v{wchh~@:Rhji[vrv{wchi~@:RhkdS&a5UY#Ta!RgPwwiI5BwciI~@:Rh`x'iJvj'!5]iJPu8Bwch]~@:Rhach)U#h3rp]gLh@t|Ax,hTq3ah!-(~@:Ro0Ou!5RhXj^v(pyw8unRhVd|)`,^UYas!a?/a2Z'a^Ta{Tb7Ta(a#!a,Wf&9sZ3DtAadamov=Bqt3[xig8vsRm~>waiL2b`{QJv*_Ouv2qgj<v]v2BqfdR'X*X#Y-@3qr~Gqv~?p6hHv-]glPup5Lq+q?_%*b_{qF{n9b^{rOu4ARhpKvCD!+&~Bqp:5Dbb}nwoiKl&unuTuBv]v+ueunaXRf0=Jvh!0nKufu8v1w&w7q%w&uHrz:Rgnj5w,uxDJq/(!hNw'5ReCk0s2u3w/w'5ReFd>Za&!*UaA=<wkgsRnSJv^!%Refifw3vyRgOKu_B'!,<]gkiiu:w&Rh<=C@a^<B57@2F{[<B5@aW:=3away9A5aW=<B=C@a^<B57@2F{Ie-#`(^^^bCara.b8aza6!/bZ,!adTbnTbOb+aFaS!aAT9@Wf~2Wli3Dtl2@d,#Y&RfnRfmJwJZtN~GqyJva&!VyMg<v~Rm%iXuJu3Rm9Jv[_=]ih9wlkDRkCd1#`(@Wg>2Wls3cH#T(@<Rj*=>Ri|b~'#23s9h<~El.d'#Y&Dtxi^rzvdRl#d*#U%(o|B2s`hJwSaxRmDKv4B&!1:Rmdd5#`'Vx}to~Hq{x'f1v3(!BA5ba|bJv_&!Wfug1v]ReIdO+U/Y#&G}-8wze=Rh{g1v]ReHg/uQRf/by#)ibQwERl/cH#T(@<Rj+=>Ri{cNu+vlax-!(#a0qa9<Rii2;;bU{H;x<i=&X#Rk`<4wwi=C9H~8xAI(Y#<azRi@45wXI<B9;5bb~7dL(X#Xa(+!aL6Vy{g5QqOau:5au2@ay547EzbxOcU(UX-T#Ta#:Cbb|A?wjh/b_|SOw6ARgtihr}u7Rhy<d1#T)X1@@=I|~=ay<2@@=aybb}Sj3vLv;<d,#Y(56H}A978H}@dGpvs@uAu`vcw9*!aFa+ai%(b!aXa8.a?a[ozWey=sU2@G}Nch&U#Rf_WexKu+D#'!t:~Gr`~?r^j]uNr|~FRg*j^psurwJt|RmcKv)@&!)7Rkv~Br[@wxfO:Rl3co#U'6Rezj_q#vIuavjRltwzeyh@vr5JqD0!>aY?C9:9au@9Cb]}9cl#U*5;5<H||jbuus1ucv&Rfvg1v~d/pppzqFr^a--a~!aMat1(hFv;Wiz@@=Izoj5uuv-7Rix~Cw`fk2WlVcZ#X,k)u3vWs@u2]ktg;wEx'fBq(_2Wg/jTv|vV~EfoJv]!15x'hzqG!(P~EfU~CRl_j6v(us5x4i-#T(2WmZ?C2F|d>Kq<aj1!*jTqIsBv=Wl`~Cw`fi2WlWj`v0u*~>RlR=c>Z,k#u3vWs@u2]kr<c1Z+jTqIsBv=Wla~Cw`fm2WlXdmb3!a{(arZa`bkTa%TbQTa-a9+c'!aM!/[yL=Bqug.w'RifhFvyDRj.g>vgwyk^9]k^Jv3_@WfbAARkhJw2_[x|JvB_wkoIRoKwkoJRoLd'(Y#<]gm=<9<H|yd'%_X#skDtb3awwqkgNulRkgdB#^',9:p'hJwSaxRmEBwVb8@4=H|qLu+w50&!)@3qs~?pU>Awwn;;Rn=c:Z'ARn<=<qwKvC@!/&~BqqJv6!&]eVb^z^xRge'/a%+^`#Sge}6<4Rn3=]n0Pw2>Rn8Jw0!&>Rn:>Rn6cY#a7+!a&=<wkaNw~h3z_c5Z{=wjh#=]nLKv^D!&)Vyz=bW|swYb<WetcG#T(2wxa@qVx@gD#Y&b^|V5JwG&!5bb|pg/w&RgD@x=kHs=uAvn!a%%/'+RmSRh694Ro`g-vaRmRhHv-]mlxCcS#`&ba~.5cD#Ta)P~=d,#Y(56H{>978H{Dd_#{2^Y%_+qbbb{6g3sERhsbU{?dfa.,`a(Xa<!aiX#(55RiG54RiHcI#T'WiU3RiVNvdwtfcRlKNvdd,#Y&RlHRlExQgf.1*^T'X#Sgf}6Wn4=]hfPrk>Rn7Jw0!&>Rn5>Rn9Lunw?&a2!,5<oq@@wqfdRlJj5Q~=d,#Y(~ARfcOuN]fdDKw;ay(}i!547E}j?cI#T(@5bV}iCbV}hdv(^^Tb?a40,b##Tbo!a*bR!a<b|a/!aKai!aU[yK=]o^g:v>ReGJwPZtK<7Rh+h<~El,Pv#5ReR@awwxjCg,ulRjDJv6&!]j!z?aQeeg>w=Sh<eeJw;!&axEzOg,Qosc!#*:wkeJ]eJ>x'h-u(!%Ro.w~h.zPdNZ(X,Ya![x{;9ReY;wkgxRiF:x?ap#Y&RmUg<s2Rkod]+UY0TZ'!a&A9sw<=bczLNvuw{gqzNhJwSaxRmCKuLay!#&s_Rf-55b^{uJvZa!!c%#(55Ri654wmiu5RiuawLu,vp!+}^%b_}Y9;wkgxba}o>A9:=b^}zKuh=a''!3awRk3c*'!#aHRk6c+Z&Rk5Rk4Jv)&!awRjSawd9*`#0?C2@EzMj8u<uJ5RmbjQrquJu3x,k>uq@_+=ayb^|W~ARkEOuN]k@7dhzV^X/X&a-#zRzSb`zXcJzTT#2WkVKvDBzW!%FzY9;5bbzWjQrquJu3Jw3%!b`zU=ayb^zQd:#X(T-a!6Vyywxh}=b]{Jg=u1RiAdGp~qHtzv!w(wA+a+a;<!aJaYai'anasb(=azRmV:Cbb{MLq2vb!%')RjuRjrRjtRjqx3jnqCw3!%')Rk(Rk+Rk&Rk)Lq2vb!%')Rj{RjxRjzRjwLq2vb!%')RjsRjpRjfRjex3jcqCw3!%')Rk'Rk*RjkRjl9<CbbzfOu4ARhxLq2vb!%')RjyRjvRjhRjgx=joq*uKvb!%')+-Rk.Rk%Rj~Rk-Rk#Rj}x=jdq*uKvb!%')+-Rk,Rk!Rj|RjmRjjRjidAq&qKs@uAv8Aa.'*-a@a&0!aM@a5[y73Dsy3Ds|3Dt):wxgI2sHJwJZt.~Gqxwsf0ikrzt}Rl0Jvy_[xj~HqzKv_A|D!&WfP8axRoVcf,U#k(v]v+ueunaXRf1Ju}'!g8u#Ri=jQw!sCunLprq>!,')~<5qeGzq9F{W=c##%s5au:5aU3CBE|;d4#X(D!a&6Vygx(b;#(=]ed?C2F{N<capoq2r[a&!aPa9,'Pw;5s:@@=I|,55w_h|@@=IzcP~=x'fCqB_2Wl2>aU@@=I|1OuNBc1Z+jTqIsBv=Wlc~Cw`fl2WlZ~AcTa%!Z+jTqIsBv=Wlb~Cw`fh2WlYk+uNqJsBv=WlSg,u3dca3#UXaMYa)TaB-=cM|7T#<bI}l5@B932:aV2G{BOuNBJq:|M!5Ezt=<B=C@a^<B57@2F{v>cB{/T#=ay<bI{3Jv6!a.6BKq0ah&+!5E}HP~Ef{978BaU@@=Iza<7d#.Y#978BaU@@=IzH~AJq0!(@@=IzG978BaU@@=IzFe,aU*Y&^^^bvJb,b:bFad!a,c2Ta>aL.bo6!a#CbTa'T#Re{2Wlh2@G{yg6t~Ro_NvdRfticuRQRllJv3&!x&c|zs@Jw3!%RflwpfkRlpKuL;%(!Re<@G|C2GzdhIvuBwgjAg-u0RjAKQB%!(GzZ@G|5NuuRl7d='T+Y#Vy[g<v~Rm!==G|>JvA!)@wma=]m1ifuaw&RmnLs@vT'!|/+[y,g:v>ReTJw1!#qX=x!eC{bLu+wT&)ZtZauq_~Graci&U#F|89:r_Lupvq!.)&2RlG8RfaC=x!eF{_h?rpWlmd&'!#X|&]k::xJey#`'T|+<E|&2@H|%dE#(^,g;u.RiEg6vjRiC9xCkA{O|zY#g=ucRmXKs0@!&*@G|m@awRknJuh!,3d(}gY}eJvj!%Rm):Jw3!%Rm+Rm-Ls0w(&!a(a#@b[|6cZ#X'7RkxWgAOu4ARn'dH'U#Y*Vz-Wm'CARm}d]*#a%^a*T'aK!a<9bV{PC=p*Jw4!&SgxcbB5r]idw(wBRmF7xFkt#&`(Rm/Rm8E|!JuY_9:Rl5=wrgr2:bbxd@xXfB(a*#T+!.X0X1Ta/a'T&RlDRfL>RlyARl9b[z[>RfZ:RlL:RfRwlg/ARl;9;RlxKv,A/!%7s69<74=BA5ba{-8Bde#`a<XaKYa1,a'P~=wxfB2bZ}}?C972@@=I}r8@55B9;5bb}G978B2@@=aybb}3j3vLv;<Jw3&!>Rfk=ayb^}4~Ad1#`*@@=aybb{w2@>==<bbz]dx+UY#^UaF!a9!bB'Ya1.!ajXa#%olRhD[y=3Dt#Ov5BrHKuMB%!(Rf^Wep~HrJwkiQjKr|~FRg)Ku+D#'!t5~GrF~?rDdV)UY,Z/_7RkuG{<~BrBg,rlsO:235B@bX}|d?a1!#`(6Vyn5@d##Y+jTv|vV~EfIj]uNpn~FRfH7Lq2vb1!a9-978BaU@@=Iz9978BbU}#~AJq0!(@@=Iz8978BaU@@=Iz7~AJQ|}!978BbU}!JvkaK!AdUa21-U#`a+(g/vsRn~Ou!5RPj:rmu9WhOjXuvvNr}:RhAj^v(pyw8unRn[kPr}p|u7vwv]RiSBd;pppzq@qHQa?(b.!a.a`@.|xa(hFv;Wiyj5uuv-7Riw~Cw`fg2WlU978BbU|wOuNBJqG!(P~EfD~CRlQcZ#X,k)u3vWs@u2]ksg;wEx'f@q1_2Wg.j]uNpn~FRfqJv]!15x'h{qG!(@@=IzK~CRl^j6v(us5x4i,#T(2WmY?C2F{1>Kq<aj1!*jTqIsBv=Wld~Cw`fj2Wl[j`v0u*~>RlT=c>Z,k#u3vWs@u2]kq<c1Z+jTqIsBv=Wle~Cw`fn2Wl]dn1#c(a(b^a2!b/bAT(bj!aDa7bu,a_a{c0!2T0g:v>ReD2@G{42@G{5~DpM~<5rc=Bx6i>{RT#RnI@zCx]y]z:2Jv[!zr5Awyk]9]k]dD(Y+X#6Vz.g=wKtgwhaCwgmTWj2Lu,w%_+/[y-B;b^xeg3u3Rj-2@bX{*KrJ<!+'@Wg(g?QRlC@Jv`!%b[zIwsfII}8JQ_@w|kW|=Jv(%!AqcOuNBJvEzh!bYzjLs@wP#(0!oy@>RkdJwMZtc3Dtd@BcG#T'9bWxg2@2Fznd*#Y+;2x'c}w<zizixNgwa#Z'U+!/!a'!a+w~g~z6wcn{Rn}wcnzRn|5Rh%=]nJg5vuRmvNvdRlvcprJu}w*az*a#!%.a.'Bot9qT]kj@Wg'ay2Gzv@Jv`!%b[zEwsfHI}1;ck#Ux`<Cbbx_Lu+w!a&0*!wko*wwo,So,}6Juqxf!E}PigQuyRm`d3(`#8>Rn%:A5B;bZ~%KvhCa!a2!x>k7#Uxb@b{#xaRk7Jw0!)>wwhlShl}6>wwhmShm}6CJvB!.x'hhvj{!!5Bwkhhbaz}x'hivjz~!5Bwkhibaz|xEhTrNu,v-vpD!a%&/)a3a.,%Ro2t[CE{)@3re9b]{%wjo09:rgc:Z&Ro6=<riifuaw&RmoKrNA!%(Ro4>Ro89;Ri`dSaL'UYzxZb)7Rka3xRhT&!,!#^1U}vbaz{>>@=be}yC@:D5bazzKu+A&!}{?ba}y>>@=be}wxBh[t`u~vJvr!%a!a()a,a0a4RoC=]o;Ju(!%RoGRhdwjh`=]oAg>w#Ro?g5vuRo=NvdRl|Ku]C.!&;RoEJvB!%RoORoMBx'h[v+_?w~h`}~5?w~hd~!xKh]oiptu-utv.vp!#%&a30a@a'a+(a/aOp(o~p!RoDJu(!%RoHRhewjha=]oBNvdRl}g>w#Ro@g5vuRo>c[#X']o<CauRoRAd-#Y':RkpauRoQKu]C.!&;RoFJvB!%RoNRoPBx'h]v+_?w~ha}t5?w~he}ue!/UbhYacXaW^Tc&a;b:a-c/#b&aja1(!cL+!bKbt!bmcRc9aIc?8[yW3Dtt94Rg`Jv}!&SiRMzBhEebShEMNuPRe>x7gL#TzuwjirRipc<Z&>on;>z=h-MSh.Mwqczx'a7vj&!>Re4@=ResJt__NuPRi*NuPRi)j]uNr|~FRfzKrJ>_+@Wfy@Wf]2WocKrJ<!+'@Wg%g/QRl@@Jv`!&awRl<wsfFIzgLu(w*!.*&ShBMwvhIRhI9;RhNx1hK'!#Sn]Mx1hK~0!#:2<H~7cNu+w7D*'1ZtW>Rn1~?rOc:Z&Rn2=<rQ<7wjh&=BSnLMc]#X(6Vz)w[b=a!U#9wzgMc3#&(RgMRitRis<x,gKt`ax!&+SioM=BSilMc3#&(RgKRinRimKurB,!&SiQMzBhDebShDM6BJQ!(P~Efx978B2@@=I}WLrJw!!,a*&@G}O@9wkibRid@@x'fKwC!&SlDMSfLMjUv~Q~EfKKv3@a+!(hFv-]mpx/hYZ(C5RiWz<o/MwkhY?So/M@x,gbvfB*&!SgEM:SoeeehFu3:Rgbda(,^TZa)X/7Sg[eb:2RgI~BrMC@wgkc:wwkcRerx3h(uUvK!&*,SnOM4Sh*MArRg;wHRh(x=h;rJvPwI!a4',a'0@Wg&=BSh/Mg>w=Rh=g3w*wwgGRgGcW(X#;Sg}M2Gzk@Jv`!&awRl=wsfGIz`dKZ*T'Y-:RhR7RhQg5u-p`j6v(us5d,#Y+~Awkia?RicOuNBwkibba}Ld6p~tyu_vbAa'a+!a/'a3aEa8a!>Sh,ebJv{!&Sh@ebSaReb9;SgwebNuPRi(NvdRl)NuPRi'hHu^<Rm^Jvv_@Wl(g;u1Si/ebKu'B&!*Sh?eb@Wl'z@aPeb95Si.ebcpputyvjB)!,&a+0a%ShAMWeK@G}C@WfJ9;RhMwvhH9w{ia}ix,hJvRA1(!zAn[MRhHx1hJ~*!#hFv(BSn[MBJQ!(@@=I~'978B2@@=I}2db.Ua<'X}+T#a0XaG2G}E;wkg|wuh!Rh!x,hZu,@)!&So0MVy)C5RiXACJvB!&5RiY5RiZg8w)cG}*T#2@bU}=KsA>(!a.3wkhZba~(x,h^u(A!&(SoCMRhb5Bz=h[eb?w~hb~6x,h_u(A!&(SoDMRhc5Bz=h]eb?w~hc~6e)aA1T#T,^^^c-bMb&blcPaP(a/!0!bA=b5c@a(!bfbrc#2afwmhARnjwchORnp2Wlf3DtsNvdRl-2@wpa<]m0bx(#:awRk2@Jw3!%RfhwpfgRlnKQB%!(G{V@G|'NuuRl6d='T+Y#VyUg<v~Rl~==G|<Jv+'!aYShC}6@B<5?ba~8@Jw3'!g2QRljhLrpWlOd+#Y'g.w'rIg>w*wgj@g-u0Rj@Lu+wT&)ZtUauq]~GrGci&U#F|39:rELrNvj!.%*RhCwunfw~nf~:9;Ri]>wtnhg;wHRnhx3hDs@v~!/+'@Wfr@9RkSNu&Rlo=@<5GzoKs0@_+@Wl+@awRkmJuh!-3d(}pY#qWJvj!%Rm(:Jw3!%Rm,Rm*de&!1U-U#`)Re;@G|.@9Ri82@wjfvRlq=@<5GzpLvOvr!).&2RlF8Rf`C=x!eE{.Jw3_g2QRlkhLrpWlPde(!#U{s,UXa*Ta'[y'g:v>ReS;x0PZ&RnlRnn~HrKJw1}f!=x!eB|2w]aP(#Xa&a*Ta.Ua2a7=]iOd'#Y&Ro&WnWg;u.RiDg6vjRiBNvdRlzhNvj]nYJuW_2Wm3x)kFze{9d])!a.!,Y01!#&aC!a3RndC=ox~BrC@2b^{pg,rlse7x'ksuq!%Rm.E{xidw(wBRmGx9o+)X#wwo-So-}69:Rl4@xSf@a#XZ'X)X,Ta(/ARl8b[xc>RfY:RlI:RfQwlg.ARl:9;Rlwdn'#^XafaQa1X1TaHTa)@b[{zcZ#X'7RkwWg@Ou4ARn&x)kG#{,g7u/RkGdH'U#Y*Vz'Wm&CARm|bx#(A]gUbUzJj9Q~=d,#Y(56H}l978H{U7d,0#U*2>ABb_xZ978BbU{e~AJQ{g!978BbU{hxMh?ad{oUYZ.x1h?{l!#:2<H{mx3n[t{vl!,&a%3Ro(z=iS}6ARnr=Bwsn^wvn`Rnbd`*T}B0!#^X'BG{c9b]{a>>@=be}F?JvS!&BG{d7BG}(Bde#`a1X,Ya@!a'P~=wxf@2bZ}I56B2@@=aybb}08@55B9;5bb}<j3vLv;<Jw3&!>Rfg=ayb^}&OuNBKuLA!)a!P~=x#fD{f2@>==<bbzl?C972@@=Ix^d6rSu,v7w*C(0a)a6#B+a%!sQ[y?3Dt%3[xn~<5rLOu!5p@Ku+D#'!t7~GrP~?rNKvlaya7'!h+v-5qMg=t|cd,U#5AAaa5Abb{S@52B5@a[@52B5Gx[iXueu;d<#`a(!/549C;ag>23ExY5@Dah89b^~689Jv)!~2b[~1Lv'w(%*!a#bX|aPrmawRe]keu7uhv-q6rxu,q`xTo]/a5aU!bNaDXbi!b-!ao!b<bwA!#5@B932:aV2G|:d-)Y#hJrL>RhG<7@C5<H|_=Cau:5aj5@B932:bJ|ng>vIbs)#?C2F|9jPv0w.vISh-MKvUaz(.!9ABbb|[5;5<H|Eg>unwfh;9:4E|YjQsBt|vjx'hYq3!(?C2F|J:2<BaY?C2F|GOu!5x,g|p{ah!-(?C2F|c9:4E|OjXuvvNr}:Rh&i[w*t|cd+U#jJvsu)vsSn~Mkfrmu9p}u7vwv]So!McW#Xa!ax5@A5aY:5;5<H|>kJv~vYrquJu3x4ib#T)2@SmZM?C2F|Bj:rmu9@xPhI(a*a#U#`a3-5Abb|L~@:RhK9:4E|0@52B5G|#C::aY?C2F|-:2<BaY?C2F|.5Jvk!a)javYrquJu3x4ia#T)2@SmYM?C2F|HAxPhH(!a#U#`a*-5Abb|4~@:RhJ9:4E|R@52B5G|F:2<BaY?C2F|Sc^#Xa2j=Qq5CJvB!-g<v{z;hhM?C2F|Zi[vrv{z;hiM?C2F|XKsA>!a)-g<v{z;h[eb?C2F|]i[vrv{z;h]eb?C2F|^iZu.vix,hZq3ah!.(?C2F|QOu!5ShXM:2<BaY?C2F|P",13494,2713,49,25,61);var be;(function(e){e[e.VALUE_LENGTH=49152]="VALUE_LENGTH",e[e.FLAG13=8192]="FLAG13",e[e.BRANCH_LENGTH=8064]="BRANCH_LENGTH",e[e.JUMP_TABLE=127]="JUMP_TABLE",e[e.VALUE_MASK=8191]="VALUE_MASK"})(be||(be={}));var De;(function(e){e[e.AMP=38]="AMP",e[e.NUM=35]="NUM",e[e.SEMI=59]="SEMI",e[e.EQUALS=61]="EQUALS",e[e.ZERO=48]="ZERO",e[e.NINE=57]="NINE",e[e.LOWER_A=97]="LOWER_A",e[e.LOWER_X=120]="LOWER_X"})(De||(De={}));var Zr=32,Tl=21,Mg=2097151,ui=2047,El=0;function Wg(e){let n=e>>>Tl;return n===ui?El:n}function Sl(e){return e-De.ZERO>>>0<=9}function Ug(e){return(e|Zr)-De.LOWER_A>>>0<=5}function Rl(e){return(e|Zr)-De.LOWER_A>>>0<=25}function Og(e){return e===De.EQUALS||Rl(e)||Sl(e)}var _l;(function(e){e[e.EntityStart=0]="EntityStart",e[e.NumericStart=1]="NumericStart",e[e.NumericDecimal=2]="NumericDecimal",e[e.NumericHex=3]="NumericHex",e[e.NamedEntity=4]="NamedEntity"})(_l||(_l={}));var ci;(function(e){e[e.Legacy=0]="Legacy",e[e.Strict=1]="Strict",e[e.Attribute=2]="Attribute"})(ci||(ci={}));function Hg(e,n,t,r){let a=(n&be.BRANCH_LENGTH)>>7,i=n&be.JUMP_TABLE;if(i){if(a===0)return r===i?t:-1;let l=r-i;if(l>>>0>=a)return-1;let u=e[t+l];return u===0?-1:t+a+u-1&65535}if(a===0)return-1;let o=a+1>>1,s=t+o+a;for(let l=0;l<a;l++){let d=e[t+(l>>1)]>>((l&1)<<3)&255;if(d===r){let c=t+o+l;return s+e[c]&65535}if(d>r)return-1}return-1}function xl(e,n,t){return t===1?String.fromCharCode(e[n]&be.VALUE_MASK):t===2?String.fromCharCode(e[n+1]):String.fromCharCode(e[n+1],e[n+2])}function jg(e,n,t){let r=n+1,a=0,i=r;if(r<t&&(e.charCodeAt(r)|Zr)===De.LOWER_X)for(r+=1,i=r;r<t;){let s=e.charCodeAt(r);if(Sl(s))a=a*16+(s-De.ZERO);else if(Ug(s))a=a*16+((s|Zr)-De.LOWER_A+10);else break;r+=1}else for(;r<t;){let s=e.charCodeAt(r)-De.ZERO;if(s>>>0>9)break;a=a*10+s,r+=1}if(r===i)return 0;r<t&&e.charCodeAt(r)===De.SEMI&&(r+=1),a>1114111&&(a=1114112);let o=r-n;return o>=ui&&(El=o,o=ui),o<<Tl|a}function $g(e,n,t){let r=yl,a=e.indexOf("&");if(a<0)return e;let i=e.length,o=0,s="",l=r[0],u=l&be.JUMP_TABLE,d=(l&be.BRANCH_LENGTH)>>7;do{let c=a+1,g=e.charCodeAt(c),h,f;if(g===De.NUM){let y=jg(e,c,i);h=Wg(y),n&&h>0&&e.charCodeAt(c+h-1)!==De.SEMI&&(h=0),f=h===0?"":ml(y&Mg)}else if(Rl(g)){h=0,f="";let y=g-u,v;if(y>>>0<d){let _=r[1+y];v=_===0?-1:d+_&65535}else v=-1;let p=0,m=0,b=v<0?0:r[v],x=c+1;e:for(;x<i;){for(;(b&(be.VALUE_LENGTH|be.FLAG13))===0&&(b&be.JUMP_TABLE)!==0;){let A=b&be.JUMP_TABLE,S=(b&be.BRANCH_LENGTH)>>7;if(S===0){if(e.charCodeAt(x)!==A)break e;v+=1}else{let k=e.charCodeAt(x)-A;if(k>>>0>=S)break e;let L=r[v+1+k];if(L===0)break e;v=v+S+L&65535}if(b=r[v],x+=1,x>=i)break e}if((b&(be.VALUE_LENGTH|be.FLAG13))===be.FLAG13){let A=(b&be.BRANCH_LENGTH)>>7;if(e.charCodeAt(x)!==(b&be.JUMP_TABLE))break;x+=1;let S=A-1,k=v+1,L=0;for(;L+1<S;L+=2){let C=r[k];if(e.charCodeAt(x)!==(C&255)||(x+=1,e.charCodeAt(x)!==(C>>8&255)))break e;x+=1,k+=1}if(L<S){if(e.charCodeAt(x)!==(r[k]&255))break;x+=1}v+=1+(A>>1),b=r[v];continue}let _=b>>>14,T=e.charCodeAt(x);if(_!==0){if(T===De.SEMI){h=x-c+1,f=_===1?String.fromCharCode(b&be.VALUE_MASK):xl(r,v,_);break}if(!n&&(b&be.FLAG13)===0&&(h=x-c,p=v,m=_),_===1)break}let R=Hg(r,b,v+(_||1),T);if(R<0)break;v=R,b=r[v],x+=1}if(f===""){let _=b>>>14;_!==0&&!n&&(b&be.FLAG13)===0&&(h=x-c,p=v,m=_),h>0&&(f=xl(r,p,m))}}else h=0,f="";h===0||t&&g!==De.NUM&&e.charCodeAt(c+h-1)!==De.SEMI&&c+h<i&&Og(e.charCodeAt(c+h))?a=c:(o<a&&(s+=e.slice(o,a)),s+=f,a=o=c+h),e.charCodeAt(a)!==De.AMP&&(a=e.indexOf("&",a))}while(a>=0);return s+e.slice(o)}function Qr(e){return $g(e,!0,!1)}var Al;(function(e){e[e.XML=0]="XML",e[e.HTML=1]="HTML"})(Al||(Al={}));var Dl;(function(e){e[e.UTF8=0]="UTF8",e[e.ASCII=1]="ASCII",e[e.Extensive=2]="Extensive",e[e.Attribute=3]="Attribute",e[e.Text=4]="Text"})(Dl||(Dl={}));var qg=class{constructor(e={}){xe(this,"src_Any",ai.source);xe(this,"src_Cc",ii.source);xe(this,"src_Z",si.source);xe(this,"src_P",jn.source);xe(this,"src_ZPCc",[this.src_Z,this.src_P,this.src_Cc].join("|"));xe(this,"src_ZCc",[this.src_Z,this.src_Cc].join("|"));xe(this,"cache",{});xe(this,"opts",{maxLength:1e4,urlAuth:!1,schema_names:[]});this.opts={...this.opts,...e}}set(e={}){return this.opts={...this.opts,...e},this.cache={},this}escapeRE(e){return e.replace(/[.?*+^$[\]\\(){}|-]/g,"\\$&")}nestedPairRE(e,n,t=4){let r=this.escapeRE(e),a=this.escapeRE(n),i=`(?:(?!${this.src_ZCc}|${r}|${a}).)`,o=`${r}${i}{0,1000}${a}`;for(let s=2;s<=t;s++)o=`${r}(?:${i}|${o}){0,1000}${a}`;return o}get_text_separators(){var e;return(e=this.cache).text_separators??(e.text_separators=/[><\uff5c]/)}get_pseudo_letter(){var e;return(e=this.cache).src_pseudo_letter??(e.src_pseudo_letter=new RegExp(`(?:(?!${this.get_text_separators().source}|${this.src_ZPCc})${this.src_Any})`))}get_ipv4_addr(){var e;return(e=this.cache).src_ip4??(e.src_ip4=new RegExp("(?:(?:25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9][0-9]|[0-9])[.]){3}(?:25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9][0-9]|[0-9])"))}get_ipv6_addr(){var t;let e="[0-9A-Fa-f]{1,4}",n=`(?:(?:${e}:${e})|${this.get_ipv4_addr().source})`;return(t=this.cache).src_ip6_addr??(t.src_ip6_addr=new RegExp(`(?:(?:${e}:){6}${n}|::(?:${e}:){5}${n}|(?:${e})?::(?:${e}:){4}${n}|(?:(?:${e}:){0,1}${e})?::(?:${e}:){3}${n}|(?:(?:${e}:){0,2}${e})?::(?:${e}:){2}${n}|(?:(?:${e}:){0,3}${e})?::${e}:${n}|(?:(?:${e}:){0,4}${e})?::${n}|(?:(?:${e}:){0,5}${e})?::${e}|(?:(?:${e}:){0,6}${e})?::)`))}get_ipv6_url_host(){var e;return(e=this.cache).src_ip6_host??(e.src_ip6_host=new RegExp(`\\[${this.get_ipv6_addr().source}\\]`))}get_ipv6_mail_host(){var e;return(e=this.cache).src_ipv6_mail_host??(e.src_ipv6_mail_host=new RegExp(`\\[IPv6:${this.get_ipv6_addr().source}\\]`))}get_auth(){var e;return(e=this.cache).src_auth??(e.src_auth=new RegExp(`(?:(?:(?!${this.src_ZCc}|[@/\\[\\]()]).){1,50}@)?`))}get_port(){var e;return(e=this.cache).src_port??(e.src_port=new RegExp("(?::(?:6(?:[0-4]\\d{3}|5(?:[0-4]\\d{2}|5(?:[0-2]\\d|3[0-5])))|[1-5]?\\d{1,4}))?"))}get_host_terminator(){var e;return(e=this.cache).src_host_terminator??(e.src_host_terminator=new RegExp(`(?=$|${this.get_text_separators().source}|${this.src_ZPCc})(?!${this.opts["---"]?"-(?!--)|":"-|"}_|:\\d|\\.-|\\.(?!$|${this.src_ZPCc}))`))}get_path_terminator(){var e;return(e=this.cache).src_path_terminator??(e.src_path_terminator=new RegExp(`${this.src_ZPCc}|${this.get_text_separators().source}`))}get_path(){var e;return(e=this.cache).src_path??(e.src_path=new RegExp(`(?:[/?#](?:${this.nestedPairRE("[","]")}|${this.nestedPairRE("(",")")}|${this.nestedPairRE("{","}")}|\\"(?:(?!${this.src_ZCc}|["]).){1,100}\\"|\\'(?:(?!${this.src_ZCc}|[']).){1,100}\\'|\\'(?=${this.get_pseudo_letter().source}|[-])|\\.{2,20}[:]?[a-zA-Z0-9%/&]|\\.(?!${this.src_ZCc}|[.]|$)|`+(this.opts["---"]?"\\-(?!--(?:[^-]|$))(?:-{0,19})|":"\\-{1,20}|")+`,(?!${this.src_ZCc}|$)|;(?!${this.src_ZCc}|$)|\\!{1,20}(?!${this.src_ZCc}|[!]|$)|\\?(?!${this.src_ZCc}|[?]|$)|`+this.get_path_extra().source+`[\\\\/:%@#&=_~*]|(?!${this.get_path_terminator().source}).){1,${this.opts.maxLength}}|\\/)?`))}get_mail_name(){var e;return(e=this.cache).src_mail_name??(e.src_mail_name=new RegExp("[-!#$%&'*+/=?^_`{|}~a-zA-Z0-9](?:[-!#$%&'*+/=?^_`{|}~a-zA-Z0-9]|[.](?=[-!#$%&'*+/=?^_`{|}~a-zA-Z0-9])){0,63}"))}get_xn(){var e;return(e=this.cache).src_xn??(e.src_xn=new RegExp("xn--[a-z0-9\\-]{1,59}"))}get_tld(){if(this.cache.tld)return this.cache.tld;let e=[...new Set(this.opts.tlds||[])].sort().reverse().join("|");return this.cache.tld=new RegExp(`${e||"$#none#$"}|${this.get_xn().source}`),this.cache.tld}get_domain_root(){var e;return(e=this.cache).src_domain_root??(e.src_domain_root=new RegExp("(?:"+this.get_xn().source+`|${this.get_pseudo_letter().source}{1,63})`))}get_domain(){var e;return(e=this.cache).src_domain??(e.src_domain=new RegExp("(?:"+this.get_xn().source+`|(?:${this.get_pseudo_letter().source})|(?:${this.get_pseudo_letter().source}(?:-|${this.get_pseudo_letter().source}){0,61}${this.get_pseudo_letter().source}))`))}get_url_host_port(){var e;return(e=this.cache).url_host_port??(e.url_host_port=new RegExp("(?:"+this.get_ipv6_url_host().source+`|(?:(?:(?:${this.get_domain().source})\\.){0,10}${this.get_domain().source}))`+this.get_port().source+this.get_host_terminator().source))}get_fuzzy_url_host_port(){var e;return(e=this.cache).fuzzy_url_host_port??(e.fuzzy_url_host_port=new RegExp("(?:"+(this.opts.fuzzyIP?this.get_ipv4_addr().source+"|":"")+`(?:(?:(?:${this.get_domain().source})\\.){1,10}(?:${this.get_tld().source})))`+this.get_host_terminator().source))}get_mail_host(){var e;return(e=this.cache).src_mail_host??(e.src_mail_host=new RegExp("(?:"+this.get_ipv6_mail_host().source+`|(?:(?:(?:${this.get_domain().source})\\.){0,4}${this.get_domain().source}))`+this.get_host_terminator().source))}get_fuzzy_mail_host(){var e;return(e=this.cache).src_fuzzy_mail_host??(e.src_fuzzy_mail_host=new RegExp("(?:"+this.get_ipv6_mail_host().source+`|(?:(?:(?:${this.get_domain().source})[.]){1,4}${this.get_domain_root().source}))`+this.get_host_terminator().source))}get_path_extra(){var e;return(e=this.cache).src_path_extra??(e.src_path_extra=new RegExp(""))}get_fuzzy_mail_host_search(){var e;return(e=this.cache).mail_fuzzy_host_search??(e.mail_fuzzy_host_search=new RegExp(`@${this.get_fuzzy_mail_host().source}`,"ig"))}get_fuzzy_link_search(){var e;return(e=this.cache).link_fuzzy_search??(e.link_fuzzy_search=new RegExp(`(^|(?![.:/\\-_@])(?:[$+<=>^\`|\uFF5C]|${this.src_ZPCc}))(?:(?![$+<=>^\`|\uFF5C])${this.get_fuzzy_url_host_port().source}${this.get_path().source})`,"ig"))}get_http_validator(){var e;return(e=this.cache).http_validator??(e.http_validator=new RegExp("\\/\\/"+(this.opts.urlAuth?this.get_auth().source:"")+this.get_url_host_port().source+this.get_path().source,"iy"))}get_relative_proto_validator(){var e;return(e=this.cache).relative_proto_validator??(e.relative_proto_validator=new RegExp((this.opts.urlAuth?this.get_auth().source:"")+`(?:localhost|${this.get_ipv6_url_host().source}|(?:(?:${this.get_domain().source})[.]){1,10}${this.get_domain_root().source})`+this.get_port().source+this.get_host_terminator().source+this.get_path().source,"iy"))}get_mail_name_validator(){var e;return(e=this.cache).mail_name_validator??(e.mail_name_validator=new RegExp(`(?:^|${this.get_text_separators().source}|"|\\(|${this.src_ZCc})(${this.get_mail_name().source})$`))}get_mailto_validator(){var e;return(e=this.cache).mailto_validator??(e.mailto_validator=new RegExp(`${this.get_mail_name().source}@${this.get_mail_host().source}`,"iy"))}get_schema_names(){var e;return(e=this.cache).schema_names??(e.schema_names=new RegExp((this.opts.schema_names||[]).map(n=>this.escapeRE(n)).join("|")))}get_schema_search(){var e;return(e=this.cache).schema_search??(e.schema_search=new RegExp(`(^|(?!_)(?:[><\uFF5C]|${this.src_ZPCc}))(${this.get_schema_names().source})`,"ig"))}get_schema_at_start(){var e;return(e=this.cache).schema_at_start??(e.schema_at_start=new RegExp(`^${this.get_schema_search().source}`,"i"))}},di={validate:(e,n,t)=>{let r=t.re.get_http_validator();r.lastIndex=n;let a=r.exec(e);return a?a[0].length:0},normalize:(e,n)=>n.normalize(e)},Xg={"http:":di,"https:":di,"ftp:":di,"//":{validate:function(e,n,t){let r=t.re.get_relative_proto_validator();r.lastIndex=n;let a=r.exec(e);return a?n>=3&&e[n-3]===":"||n>=3&&e[n-3]==="/"?0:a[0].length:0},normalize:(e,n)=>n.normalize(e)},"mailto:":{validate:function(e,n,t){let r=t.re.get_mailto_validator();r.lastIndex=n;let a=r.exec(e);return a?a[0].length:0},normalize:(e,n)=>n.normalize(e)}},Yg="a:cdefgilmnoqrstuwxz|b:abdefghijmnorstvwyz|c:acdfghiklmnoruvwxyz|d:ejkmoz|e:cegrstu|f:ijkmor|g:abdefghilmnpqrstuwy|h:kmnrtu|i:delmnoqrst|j:emop|k:eghimnprwyz|l:abcikrstuvy|m:acdeghklmnopqrstuvwxyz|n:acefgilopruz|o:m|p:aefghklmnrstwy|q:a|r:eosuw|s:abcdeghijklmnortuvxyz|t:cdfghjklmnortvwz|u:agksyz|v:aceginu|w:fs|y:et|z:amw",Jg="biz|com|edu|gov|net|org|pro|web|xxx|aero|asia|coop|info|museum|name|shop|\u0440\u0444";function Kg(){let e=Jg.split("|");return Yg.split("|").forEach(n=>{let t=n.indexOf(":"),r=n.slice(0,t);for(let a of n.slice(t+1))e.push(r+a)}),e}var Zg={fuzzyLink:!1,fuzzyEmail:!0,fuzzyIP:!1,"---":!1,tlds:Kg(),urlAuth:!1,maxLength:1e4},wl=class{constructor(e,n,t,r){xe(this,"schema");xe(this,"index");xe(this,"lastIndex");xe(this,"raw");xe(this,"text");xe(this,"url");let a=e.slice(t,r);this.schema=n.toLowerCase(),this.index=t,this.lastIndex=r,this.raw=a,this.text=a,this.url=a}},kl=class{constructor(e={}){xe(this,"__opts__");xe(this,"__schemas__");xe(this,"re");let{rebuilder:n,...t}=e;this.__opts__={...Zg,...t},this.__schemas__={...Xg},this.re=n||new qg,this.re.set({...this.__opts__,schema_names:Object.keys(this.__schemas__)})}add(e,n=null){if(!n)delete this.__schemas__[e];else{let t={normalize:(r,a)=>a.normalize(r),...n};this.__schemas__[e]=t}return this.re.set({...this.__opts__,schema_names:Object.keys(this.__schemas__)}),this}set(e={}){return this.__opts__={...this.__opts__,...e},this.re.set({...this.__opts__,schema_names:Object.keys(this.__schemas__)}),this}test(e){if(!e.length)return!1;let n,t;for(t=this.re.get_schema_search(),t.lastIndex=0;(n=t.exec(e))!==null;)if(this.testSchemaAt(e,n[2],t.lastIndex))return!0;if(this.__opts__.fuzzyLink&&this.__schemas__["http:"]&&(t=this.re.get_fuzzy_link_search(),t.lastIndex=0,t.exec(e)!==null))return!0;if(this.__opts__.fuzzyEmail&&this.__schemas__["mailto:"]&&e.indexOf("@")>=0){let r=this.re.get_fuzzy_mail_host_search(),a=this.re.get_mail_name_validator();for(r.lastIndex=0;(n=r.exec(e))!==null;){let i=e.slice(Math.max(0,n.index-65),n.index);if(a.test(i))return!0}}return!1}testSchemaAt(e,n,t){return this.__schemas__[n.toLowerCase()]?this.__schemas__[n.toLowerCase()].validate(e.slice(0,t+this.__opts__.maxLength),t,this):0}match(e){let n=[],t=this.re.get_schema_search(),r,a,i,o,s,l,u=!1,d=!1,c=!1,g=0;if(!e.length)return null;for(t.lastIndex=0,this.__opts__.fuzzyLink&&this.__schemas__["http:"]&&(r=this.re.get_fuzzy_link_search(),r.lastIndex=0),this.__opts__.fuzzyEmail&&this.__schemas__["mailto:"]&&(a=this.re.get_fuzzy_mail_host_search(),a.lastIndex=0,i=this.re.get_mail_name_validator());;){let h=Math.max(g-1,0);if(a&&i&&!c&&(!s||s.index<g))for(a.lastIndex<h&&(a.lastIndex=h);;){let m=a.exec(e);if(!m){c=!0,s=void 0;break}let b=i.exec(e.slice(Math.max(0,m.index-65),m.index));if(b){if(s={schema:"mailto:",index:m.index-b[1].length,lastIndex:m.index+m[0].length},s.index>=g)break;a.lastIndex<h&&(a.lastIndex=h)}}if(r&&!d&&(!o||o.index<g))for(r.lastIndex<h&&(r.lastIndex=h);;){let m=r.exec(e);if(!m){d=!0,o=void 0;break}if(o={schema:"",index:m.index+m[1].length,lastIndex:m.index+m[0].length},o.index>=g)break;r.lastIndex<h&&(r.lastIndex=h)}let f=s;(!f||o&&(o.index<f.index||o.index===f.index&&o.lastIndex>f.lastIndex))&&(f=o);let y;if(!u)for(;;){if(!l){t.lastIndex<h&&(t.lastIndex=h);let x=t.exec(e);if(!x){u=!0;break}l={schema:x[2],index:x.index+x[1].length,lastIndex:x.index+x[0].length}}if(l.index<g){l=void 0;continue}if(f&&l.index>f.index)break;let m=l;l=void 0;let b=this.testSchemaAt(e,m.schema,m.lastIndex);if(b){y={schema:m.schema,index:m.index,lastIndex:m.lastIndex+b};break}}let v=y;if((!v||s&&(s.index<v.index||s.index===v.index&&s.lastIndex>v.lastIndex))&&(v=s),(!v||o&&(o.index<v.index||o.index===v.index&&o.lastIndex>v.lastIndex))&&(v=o),!v)break;v===s?s=void 0:v===o&&(o=void 0);let p=new wl(e,v.schema,v.index,v.lastIndex);p.schema?this.__schemas__[p.schema].normalize(p,this):this.normalize(p),n.push(p),g=v.lastIndex}return n.length?n:null}matchAtStart(e){if(!e.length)return null;let n=this.re.get_schema_at_start().exec(e);if(!n)return null;let t=this.testSchemaAt(e,n[2],n[0].length);if(!t)return null;let r=new wl(e,n[2],n.index+n[1].length,n.index+n[0].length+t);return this.__schemas__[r.schema].normalize(r,this),r}tlds(e,n=!1){return e=Array.isArray(e)?e:[e],n?this.__opts__.tlds=this.__opts__.tlds.concat(e):this.__opts__.tlds=e,this.re.set({...this.__opts__,schema_names:Object.keys(this.__schemas__)}),this}normalize(e){e.schema||(e.url=`http://${e.url}`),e.schema==="mailto:"&&!/^mailto:/i.test(e.url)&&(e.url=`mailto:${e.url}`)}};var Qg=/^xn--/,ep=/[^\0-\x7F]/,tp=/[\x2E\u3002\uFF0E\uFF61]/g,np={overflow:"Overflow: input needs wider integers to process","not-basic":"Illegal input >= 0x80 (not a basic code point)","invalid-input":"Invalid input"},hi=35,nt=Math.floor,fi=String.fromCharCode;function St(e){throw new RangeError(np[e])}function rp(e,n){let t=[],r=e.length;for(;r--;)t[r]=n(e[r]);return t}function Fl(e,n){let t=e.split("@"),r="";t.length>1&&(r=t[0]+"@",e=t[1]),e=e.replace(tp,".");let a=e.split("."),i=rp(a,n).join(".");return r+i}function Ll(e){let n=[],t=0,r=e.length;for(;t<r;){let a=e.charCodeAt(t++);if(a>=55296&&a<=56319&&t<r){let i=e.charCodeAt(t++);(i&64512)==56320?n.push(((a&1023)<<10)+(i&1023)+65536):(n.push(a),t--)}else n.push(a)}return n}var ap=e=>String.fromCodePoint(...e),ip=function(e){return e>=48&&e<58?26+(e-48):e>=65&&e<91?e-65:e>=97&&e<123?e-97:36},Cl=function(e,n){return e+22+75*(e<26)-((n!=0)<<5)},Pl=function(e,n,t){let r=0;for(e=t?nt(e/700):e>>1,e+=nt(e/n);e>hi*26>>1;r+=36)e=nt(e/hi);return nt(r+(hi+1)*e/(e+38))},Il=function(e){let n=[],t=e.length,r=0,a=128,i=72,o=e.lastIndexOf("-");o<0&&(o=0);for(let s=0;s<o;++s)e.charCodeAt(s)>=128&&St("not-basic"),n.push(e.charCodeAt(s));for(let s=o>0?o+1:0;s<t;){let l=r;for(let d=1,c=36;;c+=36){s>=t&&St("invalid-input");let g=ip(e.charCodeAt(s++));g>=36&&St("invalid-input"),g>nt((2147483647-r)/d)&&St("overflow"),r+=g*d;let h=c<=i?1:c>=i+26?26:c-i;if(g<h)break;let f=36-h;d>nt(2147483647/f)&&St("overflow"),d*=f}let u=n.length+1;i=Pl(r-l,u,l==0),nt(r/u)>2147483647-a&&St("overflow"),a+=nt(r/u),r%=u,n.splice(r++,0,a)}return String.fromCodePoint(...n)},Gl=function(e){let n=[];e=Ll(e);let t=e.length,r=128,a=0,i=72;for(let l of e)l<128&&n.push(fi(l));let o=n.length,s=o;for(o&&n.push("-");s<t;){let l=2147483647;for(let d of e)d>=r&&d<l&&(l=d);let u=s+1;l-r>nt((2147483647-a)/u)&&St("overflow"),a+=(l-r)*u,r=l;for(let d of e)if(d<r&&++a>2147483647&&St("overflow"),d===r){let c=a;for(let g=36;;g+=36){let h=g<=i?1:g>=i+26?26:g-i;if(c<h)break;let f=c-h,y=36-h;n.push(fi(Cl(h+f%y,0))),c=nt(f/y)}n.push(fi(Cl(c,0))),i=Pl(a,u,s===o),a=0,++s}++a,++r}return n.join("")},op=function(e){return Fl(e,function(n){return Qg.test(n)?Il(n.slice(4).toLowerCase()):n})},sp=function(e){return Fl(e,function(n){return ep.test(n)?"xn--"+Gl(n):n})},lp={version:"2.3.1",ucs2:{decode:Ll,encode:ap},decode:Il,encode:Gl,toASCII:sp,toUnicode:op};var gi=lp;var zl=Object.defineProperty,Xl=(e,n)=>{let t={};for(var r in e)zl(t,r,{get:e[r],enumerable:!0});return n||zl(t,Symbol.toStringTag,{value:"Module"}),t},up=Xl({arrayReplaceAt:()=>cp,asciiTrim:()=>aa,callable:()=>Yl,escapeHtml:()=>Rt,escapeRE:()=>_p,fromCodePoint:()=>Vn,isMdAsciiPunct:()=>Yn,isPunctChar:()=>Kl,isPunctCharCode:()=>Xn,isSpace:()=>he,isValidEntityCode:()=>_i,isWhiteSpace:()=>qn,lib:()=>xp,normalizeReference:()=>ra,unescapeAll:()=>bn,unescapeMd:()=>gp});function Yl(e){let n=function(...t){return Reflect.construct(e,t,new.target&&new.target!==n?new.target:e)};return Object.defineProperty(n,"name",{value:e.name}),Object.setPrototypeOf(n,e),n.prototype=e.prototype,n}function cp(e,n,t){return[].concat(e.slice(0,n),t,e.slice(n+1))}function _i(e){return!(e>=55296&&e<=57343||e>=64976&&e<=65007||(e&65535)===65535||(e&65535)===65534||e>=0&&e<=8||e===11||e>=14&&e<=31||e>=127&&e<=159||e>1114111)}function Vn(e){if(e>65535){e-=65536;let n=55296+(e>>10),t=56320+(e&1023);return String.fromCharCode(n,t)}return String.fromCharCode(e)}var Jl=/\\([!"#$%&'()*+,\-./:;<=>?@[\\\]^_`{|}~])/g,dp=new RegExp(`${Jl.source}|${/&([a-z#][a-z0-9]{1,31});/gi.source}`,"gi"),hp=/^#((?:x[a-f0-9]{1,8}|[0-9]{1,8}))$/i;function fp(e,n){if(n.charCodeAt(0)===35&&hp.test(n)){let r=n[1].toLowerCase()==="x"?parseInt(n.slice(2),16):parseInt(n.slice(1),10);return _i(r)?Vn(r):e}let t=Qr(e);return t!==e?t:e}function gp(e){return e.indexOf("\\")<0?e:e.replace(Jl,"$1")}function bn(e){return e.indexOf("\\")<0&&e.indexOf("&")<0?e:e.replace(dp,function(n,t,r){return t||fp(n,r)})}var pp=/[&<>"]/,vp=/[&<>"]/g,mp={"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;"};function bp(e){return mp[e]}function Rt(e){return pp.test(e)?e.replace(vp,bp):e}var yp=/[.?*+^$[\]\\(){}|-]/g;function _p(e){return e.replace(yp,"\\$&")}function he(e){switch(e){case 9:case 32:return!0}return!1}function qn(e){if(e>=8192&&e<=8202)return!0;switch(e){case 9:case 10:case 11:case 12:case 13:case 32:case 160:case 5760:case 8239:case 8287:case 12288:return!0}return!1}function Kl(e){return jn.test(e)||oi.test(e)}function Xn(e){return Kl(Vn(e))}function Yn(e){switch(e){case 33:case 34:case 35:case 36:case 37:case 38:case 39:case 40:case 41:case 42:case 43:case 44:case 45:case 46:case 47:case 58:case 59:case 60:case 61:case 62:case 63:case 64:case 91:case 92:case 93:case 94:case 95:case 96:case 123:case 124:case 125:case 126:return!0;default:return!1}}function ra(e){return e=e.trim().replace(/\s+/g," "),e.toLowerCase().toUpperCase()}function Bl(e){return e===32||e===9||e===10||e===13}function aa(e){let n=0;for(;n<e.length&&Bl(e.charCodeAt(n));n++);let t=e.length-1;for(;t>=n&&Bl(e.charCodeAt(t));t--);return e.slice(n,t+1)}var xp={mdurl:ri,ucmicro:li};function Tp(e,n,t){let r,a,i,o,s=e.posMax,l=e.pos;for(e.pos=n+1,r=1;e.pos<s;){if(i=e.src.charCodeAt(e.pos),i===93&&(r--,r===0)){a=!0;break}if(o=e.pos,e.md.inline.skipToken(e),i===91){if(o===e.pos-1)r++;else if(t)return e.pos=l,-1}}let u=-1;return a&&(u=e.pos),e.pos=l,u}function Ep(e,n,t){let r,a=n,i={ok:!1,pos:0,str:""};if(e.charCodeAt(a)===60){for(a++;a<t;){if(r=e.charCodeAt(a),r===10||r===60)return i;if(r===62)return i.pos=a+1,i.str=bn(e.slice(n+1,a)),i.ok=!0,i;if(r===92&&a+1<t){a+=2;continue}a++}return i}let o=0;for(;a<t&&(r=e.charCodeAt(a),!(r===32||r<32||r===127));){if(r===92&&a+1<t){if(e.charCodeAt(a+1)===32){a++;continue}a+=2;continue}if(r===40&&(o++,o>32))return i;if(r===41){if(o===0)break;o--}a++}return n===a||o!==0||(i.str=bn(e.slice(n,a)),i.pos=a,i.ok=!0),i}function Sp(e,n,t,r){let a,i=n,o={ok:!1,can_continue:!1,pos:0,str:"",marker:0};if(r)o.str=r.str,o.marker=r.marker;else{if(i>=t)return o;let s=e.charCodeAt(i);if(s!==34&&s!==39&&s!==40)return o;n++,i++,s===40&&(s=41),o.marker=s}for(;i<t;){if(a=e.charCodeAt(i),a===o.marker)return o.pos=i+1,o.str+=bn(e.slice(n,i)),o.ok=!0,o;if(a===40&&o.marker===41)return o;a===92&&i+1<t&&i++,i++}return o.can_continue=!0,o.str+=bn(e.slice(n,i)),o}var Rp=Xl({parseLinkDestination:()=>Ep,parseLinkLabel:()=>Tp,parseLinkTitle:()=>Sp});function Jn(e){"@babel/helpers - typeof";return Jn=typeof Symbol=="function"&&typeof Symbol.iterator=="symbol"?function(n){return typeof n}:function(n){return n&&typeof Symbol=="function"&&n.constructor===Symbol&&n!==Symbol.prototype?"symbol":typeof n},Jn(e)}function Ap(e,n){if(Jn(e)!="object"||!e)return e;var t=e[Symbol.toPrimitive];if(t!==void 0){var r=t.call(e,n||"default");if(Jn(r)!="object")return r;throw new TypeError("@@toPrimitive must return a primitive value.")}return(n==="string"?String:Number)(e)}function Dp(e){var n=Ap(e,"string");return Jn(n)=="symbol"?n:n+""}function q(e,n,t){return(n=Dp(n))in e?Object.defineProperty(e,n,{value:t,enumerable:!0,configurable:!0,writable:!0}):e[n]=t,e}var Ot=class{constructor(e,n,t){q(this,"map",null),q(this,"level",0),q(this,"children",null),q(this,"content",""),q(this,"markup",""),q(this,"info",""),q(this,"block",!1),q(this,"hidden",!1),this.type=e,this.tag=n,this.attrs=null,this.nesting=t,this.meta=null}attrIndex(e){if(!this.attrs)return-1;let n=this.attrs;for(let t=0,r=n.length;t<r;t++)if(n[t][0]===e)return t;return-1}attrPush(e){this.attrs?this.attrs.push(e):this.attrs=[e]}attrSet(e,n){let t=this.attrIndex(e),r=[e,n];t<0?this.attrPush(r):this.attrs[t]=r}attrGet(e){let n=this.attrIndex(e),t=null;return n>=0&&(t=this.attrs[n][1]),t}attrJoin(e,n){let t=this.attrIndex(e);t<0?this.attrPush([e,n]):this.attrs[t][1]=`${this.attrs[t][1]} ${n}`}},Kn=class{constructor(){q(this,"__rules__",[]),q(this,"__cache__",null)}__find__(e){for(let n=0;n<this.__rules__.length;n++)if(this.__rules__[n].name===e)return n;return-1}__compile__(){let e=new Set;this.__rules__.forEach(n=>{n.enabled&&n.alt.forEach(t=>{t&&e.add(t)})}),this.__cache__=Object.create(null),this.__cache__[""]=[],this.__rules__.forEach(n=>{n.enabled&&this.__cache__[""].push(n.fn)}),e.forEach(n=>{this.__cache__[n]=[],this.__rules__.forEach(t=>{t.enabled&&t.alt.indexOf(n)>=0&&this.__cache__[n].push(t.fn)})})}at(e,n,t={}){let r=this.__find__(e);if(r===-1)throw new Error(`Parser rule not found: ${e}`);this.__rules__[r].fn=n,this.__rules__[r].alt=t.alt||[],this.__cache__=null}before(e,n,t,r={}){let a=this.__find__(e);if(a===-1)throw new Error(`Parser rule not found: ${e}`);this.__rules__.splice(a,0,{name:n,enabled:!0,fn:t,alt:r.alt||[]}),this.__cache__=null}after(e,n,t,r={}){let a=this.__find__(e);if(a===-1)throw new Error(`Parser rule not found: ${e}`);this.__rules__.splice(a+1,0,{name:n,enabled:!0,fn:t,alt:r.alt||[]}),this.__cache__=null}push(e,n,t={}){this.__rules__.push({name:e,enabled:!0,fn:n,alt:t.alt||[]}),this.__cache__=null}enable(e,n=!1){Array.isArray(e)||(e=[e]);let t=[];return e.forEach(r=>{let a=this.__find__(r);if(a<0){if(n)return;throw new Error(`Rules manager: invalid rule name ${r}`)}this.__rules__[a].enabled=!0,t.push(r)}),this.__cache__=null,t}enableOnly(e,n=!1){Array.isArray(e)||(e=[e]),this.__rules__.forEach(t=>{t.enabled=!1}),this.enable(e,n)}disable(e,n=!1){Array.isArray(e)||(e=[e]);let t=[];return e.forEach(r=>{let a=this.__find__(r);if(a<0){if(n)return;throw new Error(`Rules manager: invalid rule name ${r}`)}this.__rules__[a].enabled=!1,t.push(r)}),this.__cache__=null,t}getRules(e){return this.__cache__||this.__compile__(),this.__cache__[e]||[]}},rt={};rt.code_inline=function(e,n,t,r,a){let i=e[n];return`<code${a.renderAttrs(i)}>${Rt(i.content)}</code>`};rt.code_block=function(e,n,t,r,a){let i=e[n];return`<pre${a.renderAttrs(i)}><code>${Rt(e[n].content)}</code></pre>
`};rt.fence=function(e,n,t,r,a){let i=e[n],o=i.info?bn(i.info).trim():"",s="",l="";if(o){let d=o.split(/(\s+)/g);s=d[0],l=d.slice(2).join("")}let u;if(t.highlight?u=t.highlight(i.content,s,l)||Rt(i.content):u=Rt(i.content),u.indexOf("<pre")===0)return u+`
`;if(o){let d=i.attrIndex("class"),c=i.attrs?i.attrs.slice():[];d<0?c.push(["class",`${t.langPrefix}${s}`]):(c[d]=[c[d][0],c[d][1]],c[d][1]+=` ${t.langPrefix}${s}`);let g={attrs:c};return`<pre><code${a.renderAttrs(g)}>${u}</code></pre>
`}return`<pre><code${a.renderAttrs(i)}>${u}</code></pre>
`};rt.image=function(e,n,t,r,a){let i=e[n];return i.attrs[i.attrIndex("alt")][1]=a.renderInlineAsText(i.children,t,r),a.renderToken(e,n,t)};rt.hardbreak=function(e,n,t){return t.xhtmlOut?`<br />
`:`<br>
`};rt.softbreak=function(e,n,t){return t.breaks?t.xhtmlOut?`<br />
`:`<br>
`:`
`};rt.text=function(e,n){return Rt(e[n].content)};rt.html_block=function(e,n){return e[n].content};rt.html_inline=function(e,n){return e[n].content};var Zl=class{constructor(){q(this,"rules",Object.assign({},rt))}renderAttrs(e){let n,t,r;if(!e.attrs)return"";for(r="",n=0,t=e.attrs.length;n<t;n++)r+=` ${Rt(e.attrs[n][0])}="${Rt(String(e.attrs[n][1]))}"`;return r}renderToken(e,n,t){let r=e[n],a="";if(r.hidden)return"";let i=n-1;for(;i>=0&&e[i].hidden&&e[i].nesting===0;)i--;r.block&&r.nesting!==-1&&i>=0&&e[i].hidden&&e[i].nesting===-1&&(a+=`
`),a+=(r.nesting===-1?"</":"<")+r.tag,a+=this.renderAttrs(r),r.nesting===0&&t.xhtmlOut&&(a+=" /");let o=!1;if(r.block&&(o=!0,r.nesting===1)){let s=n+1;for(;s<e.length&&e[s].hidden&&e[s].nesting===0;)s++;if(s<e.length){let l=e[s];(l.type==="inline"||l.hidden||l.nesting===-1&&l.tag===r.tag)&&(o=!1)}}return a+=o?`>
`:">",a}renderInline(e,n,t){let r="",a=this.rules;for(let i=0,o=e.length;i<o;i++){let s=e[i].type;typeof a[s]<"u"?r+=a[s](e,i,n,t,this):r+=this.renderToken(e,i,n)}return r}renderInlineAsText(e,n,t){let r="";for(let a=0,i=e.length;a<i;a++)switch(e[a].type){case"text":case"code_inline":r+=e[a].content;break;case"image":r+=this.renderInlineAsText(e[a].children,n,t);break;case"html_inline":case"html_block":r+=e[a].content;break;case"softbreak":case"hardbreak":r+=`
`}return r}render(e,n,t){let r="",a=this.rules;for(let i=0,o=e.length;i<o;i++){let s=e[i].type;s==="inline"?r+=this.renderInline(e[i].children,n,t):typeof a[s]<"u"?r+=a[s](e,i,n,t,this):r+=this.renderToken(e,i,n)}return r}},Ql=class{constructor(e,n,t){q(this,"tokens",[]),q(this,"inlineMode",!1),q(this,"Token",Ot),this.src=e,this.env=t,this.md=n}},wp=/\r\n?/g,kp=/\0/g;function Cp(e){let n;n=e.src.replace(wp,`
`),n=n.replace(kp,"\uFFFD"),e.src=n}function Fp(e){let n;e.inlineMode?(n=new e.Token("inline","",0),n.content=e.src,n.map=[0,1],n.children=[],e.tokens.push(n)):e.md.block.parse(e.src,e.md,e.env,e.tokens)}function Lp(e){let n=e.tokens,t=0;for(let r=0;r<n.length;r++)n[r].type!=="reference_definition"&&(r!==t&&(n[t]=n[r]),t++);n.length!==t&&(n.length=t)}function Pp(e){let n=e.tokens;for(let t=0,r=n.length;t<r;t++){let a=n[t];a.type==="inline"&&e.md.inline.parse(a.content,e.md,e.env,a.children)}}function Ip(e){return/^<a[>\s]/i.test(e)}function Gp(e){return/^<\/a\s*>/i.test(e)}function zp(e){let n=e.tokens;if(e.md.options.linkify)for(let t=0,r=n.length;t<r;t++){if(n[t].type!=="inline"||!e.md.linkify.test(n[t].content))continue;let a=n[t].children,i=[],o=0;for(let s=a.length-1;s>=0;s--){let l=a[s];if(l.type==="link_close"){for(s--;a[s].level!==l.level&&a[s].type!=="link_open";)s--;continue}if(l.type==="html_inline"&&(Ip(l.content)&&o>0&&o--,Gp(l.content)&&o++),!(o>0)&&l.type==="text"&&e.md.linkify.test(l.content)){let u=l.content,d=e.md.linkify.match(u),c=[],g=l.level,h=0;d.length>0&&d[0].index===0&&s>0&&a[s-1].type==="text_special"&&(d=d.slice(1));for(let f=0;f<d.length;f++){let y=d[f].url,v=e.md.normalizeLink(y);if(!e.md.validateLink(v))continue;let p=d[f].text;d[f].schema?d[f].schema==="mailto:"&&!/^mailto:/i.test(p)?p=e.md.normalizeLinkText(`mailto:${p}`).replace(/^mailto:/,""):p=e.md.normalizeLinkText(p):p=e.md.normalizeLinkText(`http://${p}`).replace(/^http:\/\//,"");let m=d[f].index;if(m>h){let T=new e.Token("text","",0);T.content=u.slice(h,m),T.level=g,c.push(T)}let b=new e.Token("link_open","a",1);b.attrs=[["href",v]],b.level=g++,b.markup="linkify",b.info="auto",c.push(b);let x=new e.Token("text","",0);x.content=p,x.level=g,c.push(x);let _=new e.Token("link_close","a",-1);_.level=--g,_.markup="linkify",_.info="auto",c.push(_),h=d[f].lastIndex}if(h<u.length){let f=new e.Token("text","",0);f.content=u.slice(h),f.level=g,c.push(f)}i.push({index:s,nodes:c})}}if(i.length>0){let s=a.length;for(let c of i)s+=c.nodes.length-1;let l=new Array(s),u=0,d=0;i.reverse();for(let c=0;c<a.length;c++){let g=i[u];if(g?.index===c){for(let h of g.nodes)l[d++]=h;u++}else l[d++]=a[c]}n[t].children=l}}}var eu=/\+-|\.\.|\?\?\?\?|!!!!|,,|--/,Bp=/\((c|tm|r)\)/i,Np=/\((c|tm|r)\)/gi,Mp={c:"\xA9",r:"\xAE",tm:"\u2122"};function Wp(e,n){return Mp[n.toLowerCase()]}function Up(e){let n=0;for(let t=e.length-1;t>=0;t--){let r=e[t];r.type==="text"&&!n&&(r.content=r.content.replace(Np,Wp)),r.type==="link_open"&&r.info==="auto"&&n--,r.type==="link_close"&&r.info==="auto"&&n++}}function Op(e){let n=0;for(let t=e.length-1;t>=0;t--){let r=e[t];r.type==="text"&&!n&&eu.test(r.content)&&(r.content=r.content.replace(/\+-/g,"\xB1").replace(/\.{2,}/g,"\u2026").replace(/([?!])…/g,"$1..").replace(/([?!]){4,}/g,"$1$1$1").replace(/,{2,}/g,",").replace(/(^|[^-])---(?=[^-]|$)/gm,"$1\u2014").replace(/(^|\s)--(?=\s|$)/gm,"$1\u2013").replace(/(^|[^-\s])--(?=[^-\s]|$)/gm,"$1\u2013")),r.type==="link_open"&&r.info==="auto"&&n--,r.type==="link_close"&&r.info==="auto"&&n++}}function Hp(e){let n;if(e.md.options.typographer)for(n=e.tokens.length-1;n>=0;n--)e.tokens[n].type==="inline"&&(Bp.test(e.tokens[n].content)&&Up(e.tokens[n].children),eu.test(e.tokens[n].content)&&Op(e.tokens[n].children))}var jp=/['"]/,Nl=/['"]/g,Ml="\u2019",$p=1e3;function Wl(e,n,t){for(;e.length>t;){let r=e.pop();r.isSingleQuote?n.single=r.prevSameQuoteIdx:n.double=r.prevSameQuoteIdx}}function ea(e,n,t,r){e[n]||(e[n]=[]),e[n].push({pos:t,ch:r})}function Vp(e,n){let t="",r=0;n.sort((a,i)=>a.pos-i.pos);for(let a=0;a<n.length;a++){let i=n[a];t+=e.slice(r,i.pos)+i.ch,r=i.pos+1}return t+e.slice(r)}function qp(e,n){let t,r=[],a={single:-1,double:-1},i={};for(let o=0;o<e.length;o++){let s=e[o],l=e[o].level;for(t=r.length-1;t>=0&&!(r[t].level<=l);t--);if(Wl(r,a,t+1),s.type!=="text")continue;let u=s.content,d=0,c=u.length;e:for(;d<c;){Nl.lastIndex=d;let g=Nl.exec(u);if(!g)break;let h=!0,f=!0;d=g.index+1;let y=g[0]==="'",v=32;if(g.index-1>=0)v=u.charCodeAt(g.index-1);else for(t=o-1;t>=0&&!(e[t].type==="softbreak"||e[t].type==="hardbreak");t--)if(e[t].content){v=e[t].content.charCodeAt(e[t].content.length-1);break}let p=32;if(d<c)p=u.charCodeAt(d);else for(t=o+1;t<e.length&&!(e[t].type==="softbreak"||e[t].type==="hardbreak");t++)if(e[t].content){p=e[t].content.charCodeAt(0);break}let m=Yn(v)||Xn(v),b=Yn(p)||Xn(p),x=qn(v),_=qn(p);if(_?h=!1:b&&(x||m||(h=!1)),x?f=!1:m&&(_||b||(f=!1)),p===34&&g[0]==='"'&&v>=48&&v<=57&&(f=h=!1),h&&f&&(h=m,f=b),!h&&!f){y&&ea(i,o,g.index,Ml);continue}if(f&&(t=y?a.single:a.double,t>=0&&r[t].level===l)){let T=r[t],R,A;y?(R=n.md.options.quotes[2],A=n.md.options.quotes[3]):(R=n.md.options.quotes[0],A=n.md.options.quotes[1]),ea(i,o,g.index,A),ea(i,T.tokenIdx,T.contentPos,R),Wl(r,a,t);continue e}if(h){if(r.length>=$p)return;r.push({tokenIdx:o,contentPos:g.index,isSingleQuote:y,level:l,prevSameQuoteIdx:y?a.single:a.double}),y?a.single=r.length-1:a.double=r.length-1}else f&&y&&ea(i,o,g.index,Ml)}}Object.keys(i).forEach(function(o){let s=Number(o);e[s].content=Vp(e[s].content,i[o])})}function Xp(e){if(e.md.options.typographer)for(let n=e.tokens.length-1;n>=0;n--)e.tokens[n].type!=="inline"||!jp.test(e.tokens[n].content)||qp(e.tokens[n].children,e)}function Yp(e){let n,t,r=e.length;for(n=0;n<r;n++)e[n].type==="text_special"&&(e[n].type="text");for(n=t=0;n<r;n++)e[n].type==="text"&&n+1<r&&e[n+1].type==="text"?e[n+1].content=e[n].content+e[n+1].content:(n!==t&&(e[t]=e[n]),t++);n!==t&&(e.length=t)}function Jp(e){let n,t,r=e.tokens,a=r.length;for(let i=0;i<a;i++){if(r[i].type!=="inline")continue;let o=r[i].children,s=o.length;for(n=0;n<s;n++)o[n].type==="text_special"&&(o[n].type="text"),o[n].children&&Yp(o[n].children);for(n=t=0;n<s;n++)o[n].type==="text"&&n+1<s&&o[n+1].type==="text"?o[n+1].content=o[n].content+o[n+1].content:(n!==t&&(o[t]=o[n]),t++);n!==t&&(o.length=t)}}var pi=[["normalize",Cp],["block",Fp],["strip_references",Lp],["inline",Pp],["linkify",zp],["replacements",Hp],["smartquotes",Xp],["text_join",Jp]],tu=class{constructor(){q(this,"ruler",new Kn),q(this,"State",Ql);for(let e=0;e<pi.length;e++)this.ruler.push(pi[e][0],pi[e][1])}process(e){let n=this.ruler.getRules("");for(let t=0,r=n.length;t<r;t++)n[t](e)}},nu=class{constructor(e,n,t,r){q(this,"bMarks",[]),q(this,"eMarks",[]),q(this,"tShift",[]),q(this,"sCount",[]),q(this,"bsCount",[]),q(this,"blkIndent",0),q(this,"line",0),q(this,"lineMax",0),q(this,"tight",!1),q(this,"listIndent",-1),q(this,"parentType","root"),q(this,"level",0),q(this,"Token",Ot),this.src=e,this.md=n,this.env=t,this.tokens=r;let a=this.src;for(let i=0,o=0,s=0,l=0,u=a.length,d=!1;o<u;o++){let c=a.charCodeAt(o);if(!d)if(he(c)){s++,c===9?l+=4-l%4:l++;continue}else d=!0;(c===10||o===u-1)&&(c!==10&&o++,this.bMarks.push(i),this.eMarks.push(o),this.tShift.push(s),this.sCount.push(l),this.bsCount.push(0),d=!1,s=0,l=0,i=o+1)}this.bMarks.push(a.length),this.eMarks.push(a.length),this.tShift.push(0),this.sCount.push(0),this.bsCount.push(0),this.lineMax=this.bMarks.length-1}push(e,n,t){let r=new Ot(e,n,t);return r.block=!0,t<0&&this.level--,r.level=this.level,t>0&&this.level++,this.tokens.push(r),r}isEmpty(e){return this.bMarks[e]+this.tShift[e]>=this.eMarks[e]}skipEmptyLines(e){for(let n=this.lineMax;e<n&&!(this.bMarks[e]+this.tShift[e]<this.eMarks[e]);e++);return e}skipSpaces(e){for(let n=this.src.length;e<n&&he(this.src.charCodeAt(e));e++);return e}skipSpacesBack(e,n){if(e<=n)return e;for(;e>n;)if(!he(this.src.charCodeAt(--e)))return e+1;return e}skipChars(e,n){for(let t=this.src.length;e<t&&this.src.charCodeAt(e)===n;e++);return e}skipCharsBack(e,n,t){if(e<=t)return e;for(;e>t;)if(n!==this.src.charCodeAt(--e))return e+1;return e}getLines(e,n,t,r){if(e>=n)return"";let a=new Array(n-e);for(let i=0,o=e;o<n;o++,i++){let s=0,l=this.bMarks[o],u=l,d;for(o+1<n||r?d=this.eMarks[o]+1:d=this.eMarks[o];u<d&&s<t;){let c=this.src.charCodeAt(u);if(he(c))c===9?s+=4-(s+this.bsCount[o])%4:s++;else if(u-l<this.tShift[o])s++;else break;u++}s>t?a[i]=new Array(s-t+1).join(" ")+this.src.slice(u,d):a[i]=this.src.slice(u,d)}return a.join("")}},Kp=65536;function vi(e,n){let t=e.bMarks[n]+e.tShift[n],r=e.eMarks[n];return e.src.slice(t,r)}function Ul(e){let n=[],t=e.length,r=0,a=e.charCodeAt(r),i=!1,o=0,s="";for(;r<t;)a===124&&(i?(s+=e.substring(o,r-1),o=r):(n.push(s+e.substring(o,r)),s="",o=r+1)),i=a===92,r++,a=e.charCodeAt(r);return n.push(s+e.substring(o)),n}function Zp(e,n,t,r){if(n+2>t)return!1;let a=n+1;if(e.sCount[a]<e.blkIndent||e.sCount[a]-e.blkIndent>=4)return!1;let i=e.bMarks[a]+e.tShift[a];if(i>=e.eMarks[a])return!1;let o=e.src.charCodeAt(i++);if(o!==124&&o!==45&&o!==58||i>=e.eMarks[a])return!1;let s=e.src.charCodeAt(i++);if(s!==124&&s!==45&&s!==58&&!he(s)||o===45&&he(s))return!1;for(;i<e.eMarks[a];){let x=e.src.charCodeAt(i);if(x!==124&&x!==45&&x!==58&&!he(x))return!1;i++}let l=vi(e,n+1),u=l.split("|"),d=[];for(let x=0;x<u.length;x++){let _=u[x].trim();if(!_){if(x===0||x===u.length-1)continue;return!1}if(!/^:?-+:?$/.test(_))return!1;_.charCodeAt(_.length-1)===58?d.push(_.charCodeAt(0)===58?"center":"right"):_.charCodeAt(0)===58?d.push("left"):d.push("")}if(l=vi(e,n).trim(),l.indexOf("|")===-1||e.sCount[n]-e.blkIndent>=4)return!1;u=Ul(l),u.length&&u[0]===""&&u.shift(),u.length&&u[u.length-1]===""&&u.pop();let c=u.length;if(c===0||c!==d.length)return!1;if(r)return!0;let g=e.parentType;e.parentType="table";let h=e.md.block.ruler.getRules("blockquote"),f=e.push("table_open","table",1),y=[n,0];f.map=y;let v=e.push("thead_open","thead",1);v.map=[n,n+1];let p=e.push("tr_open","tr",1);p.map=[n,n+1];for(let x=0;x<u.length;x++){let _=e.push("th_open","th",1);d[x]&&(_.attrs=[["style",`text-align:${d[x]}`]]);let T=e.push("inline","",0);T.content=u[x].trim(),T.children=[],e.push("th_close","th",-1)}e.push("tr_close","tr",-1),e.push("thead_close","thead",-1);let m,b=0;for(a=n+2;a<t&&!(e.sCount[a]<e.blkIndent);a++){let x=!1;for(let T=0,R=h.length;T<R;T++)if(h[T](e,a,t,!0)){x=!0;break}if(x||(l=vi(e,a).trim(),!l)||e.sCount[a]-e.blkIndent>=4||(u=Ul(l),u.length&&u[0]===""&&u.shift(),u.length&&u[u.length-1]===""&&u.pop(),b+=c-u.length,b>Kp))break;if(a===n+2){let T=e.push("tbody_open","tbody",1);T.map=m=[n+2,0]}let _=e.push("tr_open","tr",1);_.map=[a,a+1];for(let T=0;T<c;T++){let R=e.push("td_open","td",1);d[T]&&(R.attrs=[["style",`text-align:${d[T]}`]]);let A=e.push("inline","",0);A.content=u[T]?u[T].trim():"",A.children=[],e.push("td_close","td",-1)}e.push("tr_close","tr",-1)}return m&&(e.push("tbody_close","tbody",-1),m[1]=a),e.push("table_close","table",-1),y[1]=a,e.parentType=g,e.line=a,!0}function Qp(e,n,t){if(e.sCount[n]-e.blkIndent<4)return!1;let r=n+1,a=r;for(;r<t;){if(e.isEmpty(r)){r++;continue}if(e.sCount[r]-e.blkIndent>=4){r++,a=r;continue}break}e.line=a;let i=e.push("code_block","code",0);return i.content=e.getLines(n,a,4+e.blkIndent,!1)+`
`,i.map=[n,e.line],!0}function ev(e,n,t,r){let a=e.bMarks[n]+e.tShift[n],i=e.eMarks[n];if(e.sCount[n]-e.blkIndent>=4||a+3>i)return!1;let o=e.src.charCodeAt(a);if(o!==126&&o!==96)return!1;let s=a;a=e.skipChars(a,o);let l=a-s;if(l<3)return!1;let u=e.src.slice(s,a),d=e.src.slice(a,i);if(o===96&&d.indexOf(String.fromCharCode(o))>=0)return!1;if(r)return!0;let c=n,g=!1;for(;c++,!(c>=t||(a=s=e.bMarks[c]+e.tShift[c],i=e.eMarks[c],a<i&&e.sCount[c]<e.blkIndent));)if(e.src.charCodeAt(a)===o&&!(e.sCount[c]-e.blkIndent>=4)&&(a=e.skipChars(a,o),!(a-s<l)&&(a=e.skipSpaces(a),!(a<i)))){g=!0;break}l=e.sCount[n],e.line=c+(g?1:0);let h=e.push("fence","code",0);return h.info=d,h.content=e.getLines(n+1,c,l,!0),h.markup=u,h.map=[n,e.line],!0}function tv(e,n,t,r){let a=e.bMarks[n]+e.tShift[n],i=e.eMarks[n],o=e.lineMax;if(e.sCount[n]-e.blkIndent>=4||e.src.charCodeAt(a)!==62)return!1;if(r)return!0;let s=[],l=[],u=[],d=[],c=e.md.block.ruler.getRules("blockquote"),g=e.parentType;e.parentType="blockquote";let h=!1,f;for(f=n;f<t;f++){let b=e.sCount[f]<e.blkIndent;if(a=e.bMarks[f]+e.tShift[f],i=e.eMarks[f],a>=i)break;if(e.src.charCodeAt(a++)===62&&!b){let _=e.sCount[f]+1,T,R;e.src.charCodeAt(a)===32?(a++,_++,R=!1,T=!0):e.src.charCodeAt(a)===9?(T=!0,(e.bsCount[f]+_)%4===3?(a++,_++,R=!1):R=!0):T=!1;let A=_;for(s.push(e.bMarks[f]),e.bMarks[f]=a;a<i;){let S=e.src.charCodeAt(a);if(he(S))S===9?A+=4-(A+e.bsCount[f]+(R?1:0))%4:A++;else break;a++}h=a>=i,l.push(e.bsCount[f]),e.bsCount[f]=e.sCount[f]+1+(T?1:0),u.push(e.sCount[f]),e.sCount[f]=A-_,d.push(e.tShift[f]),e.tShift[f]=a-e.bMarks[f];continue}if(h)break;let x=!1;for(let _=0,T=c.length;_<T;_++)if(c[_](e,f,t,!0)){x=!0;break}if(x){e.lineMax=f,e.blkIndent!==0&&(s.push(e.bMarks[f]),l.push(e.bsCount[f]),d.push(e.tShift[f]),u.push(e.sCount[f]),e.sCount[f]-=e.blkIndent);break}s.push(e.bMarks[f]),l.push(e.bsCount[f]),d.push(e.tShift[f]),u.push(e.sCount[f]),e.sCount[f]=-1}let y=e.blkIndent;e.blkIndent=0;let v=e.push("blockquote_open","blockquote",1);v.markup=">";let p=[n,0];v.map=p,e.md.block.tokenize(e,n,f);let m=e.push("blockquote_close","blockquote",-1);m.markup=">",e.lineMax=o,e.parentType=g,p[1]=e.line;for(let b=0;b<d.length;b++)e.bMarks[b+n]=s[b],e.tShift[b+n]=d[b],e.sCount[b+n]=u[b],e.bsCount[b+n]=l[b];return e.blkIndent=y,!0}function nv(e,n,t,r){let a=e.eMarks[n];if(e.sCount[n]-e.blkIndent>=4)return!1;let i=e.bMarks[n]+e.tShift[n],o=e.src.charCodeAt(i++);if(o!==42&&o!==45&&o!==95)return!1;let s=1;for(;i<a;){let u=e.src.charCodeAt(i++);if(u!==o&&!he(u))return!1;u===o&&s++}if(s<3)return!1;if(r)return!0;e.line=n+1;let l=e.push("hr","hr",0);return l.map=[n,e.line],l.markup=Array(s+1).join(String.fromCharCode(o)),!0}function Ol(e,n){let t=e.eMarks[n],r=e.bMarks[n]+e.tShift[n],a=e.src.charCodeAt(r++);return a!==42&&a!==45&&a!==43||r<t&&!he(e.src.charCodeAt(r))?-1:r}function Hl(e,n){let t=e.bMarks[n]+e.tShift[n],r=e.eMarks[n],a=t;if(a+1>=r)return-1;let i=e.src.charCodeAt(a++);if(i<48||i>57)return-1;for(;;){if(a>=r)return-1;if(i=e.src.charCodeAt(a++),i>=48&&i<=57){if(a-t>=10)return-1;continue}if(i===41||i===46)break;return-1}return a<r&&(i=e.src.charCodeAt(a),!he(i))?-1:a}function rv(e,n){let t=e.level+2;for(let r=n+2,a=e.tokens.length-2;r<a;r++)e.tokens[r].level===t&&e.tokens[r].type==="paragraph_open"&&(e.tokens[r+2].hidden=!0,e.tokens[r].hidden=!0,r+=2)}function av(e,n,t,r){let a,i,o,s,l=n,u=!0;if(e.sCount[l]-e.blkIndent>=4||e.listIndent>=0&&e.sCount[l]-e.listIndent>=4&&e.sCount[l]<e.blkIndent)return!1;let d=!1;r&&e.parentType==="paragraph"&&e.sCount[l]>=e.blkIndent&&(d=!0);let c,g,h;if((h=Hl(e,l))>=0){if(c=!0,o=e.bMarks[l]+e.tShift[l],g=Number(e.src.slice(o,h-1)),d&&g!==1)return!1}else if((h=Ol(e,l))>=0)c=!1;else return!1;if(d&&e.skipSpaces(h)>=e.eMarks[l])return!1;if(r)return!0;let f=e.src.charCodeAt(h-1),y=e.tokens.length;c?(s=e.push("ordered_list_open","ol",1),g!==1&&(s.attrs=[["start",g]])):s=e.push("bullet_list_open","ul",1);let v=[l,0];s.map=v,s.markup=String.fromCharCode(f);let p=!1,m=e.md.block.ruler.getRules("list"),b=e.parentType;for(e.parentType="list";l<t;){i=h,a=e.eMarks[l];let x=e.sCount[l]+h-(e.bMarks[l]+e.tShift[l]),_=x;for(;i<a;){let P=e.src.charCodeAt(i);if(P===9)_+=4-(_+e.bsCount[l])%4;else if(P===32)_++;else break;i++}let T=i,R;T>=a?R=1:R=_-x,R>4&&(R=1);let A=x+R;s=e.push("list_item_open","li",1),s.markup=String.fromCharCode(f);let S=[l,0];s.map=S,c&&(s.info=e.src.slice(o,h-1));let k=e.tight,L=e.tShift[l],C=e.sCount[l],z=e.listIndent;if(e.listIndent=e.blkIndent,e.blkIndent=A,e.tight=!0,e.tShift[l]=T-e.bMarks[l],e.sCount[l]=_,T>=a&&e.isEmpty(l+1)?e.line=Math.min(e.line+2,t):e.md.block.tokenize(e,l,t),(!e.tight||p)&&(u=!1),p=e.line-l>1&&e.isEmpty(e.line-1),e.blkIndent=e.listIndent,e.listIndent=z,e.tShift[l]=L,e.sCount[l]=C,e.tight=k,s=e.push("list_item_close","li",-1),s.markup=String.fromCharCode(f),l=e.line,S[1]=l,l>=t||e.sCount[l]<e.blkIndent||e.sCount[l]-e.blkIndent>=4)break;let F=!1;for(let P=0,N=m.length;P<N;P++)if(m[P](e,l,t,!0)){F=!0;break}if(F)break;if(c){if(h=Hl(e,l),h<0)break;o=e.bMarks[l]+e.tShift[l]}else if(h=Ol(e,l),h<0)break;if(f!==e.src.charCodeAt(h-1))break}return c?s=e.push("ordered_list_close","ol",-1):s=e.push("bullet_list_close","ul",-1),s.markup=String.fromCharCode(f),v[1]=l,e.line=l,e.parentType=b,u&&rv(e,y),!0}var mi=999;function iv(e){let n=e.length;for(let t=0;t<e.length-1;t++){let r=e.charCodeAt(t);if(r<55296||r>56319)continue;let a=e.charCodeAt(t+1);a>=56320&&a<=57343&&(n--,t++)}return n}function $n(e,n){return Math.min(e.eMarks[n]+1,e.src.length)}function ta(e,n){let t=e.lineMax;if(n>=t||e.isEmpty(n))return-1;let r=!1;if(e.sCount[n]-e.blkIndent>3&&(r=!0),e.sCount[n]<0&&(r=!0),!r){let a=e.md.block.ruler.getRules("reference"),i=e.parentType;e.parentType="reference";let o=!1;for(let s=0,l=a.length;s<l;s++)if(a[s](e,n,t,!0)){o=!0;break}if(e.parentType=i,o)return-1}return e.bMarks[n]+e.tShift[n]}function ov(e,n,t,r){let a=e.src,i=e.bMarks[n]+e.tShift[n],o=n+1;if(e.sCount[n]-e.blkIndent>=4||a.charCodeAt(i)!==91)return!1;let s=$n(e,n),l="",u=-1,d=++i;for(;i<s;){if(l.length+i-d>mi*2)return!1;let T=a.charCodeAt(i);if(T===91)return!1;if(T===93){u=i;break}if(T===92&&(T=a.charCodeAt(++i)),T===10&&i<s){let R=ta(e,o);if(R<0)return!1;l+=a.slice(d,s),i=d=R,s=$n(e,o++);continue}i++}if(u<0||a.charCodeAt(u+1)!==58||(l+=a.slice(d,u),l.length>mi&&iv(l)>mi))return!1;for(i=u+2;i<s;){let T=a.charCodeAt(i);if(T===10){let R=ta(e,o);if(R<0)break;i=R,s=$n(e,o++)}else if(he(T))i++;else break}let c=e.md.helpers.parseLinkDestination(a,i,s);if(!c.ok)return!1;let g=e.md.normalizeLink(c.str);if(!e.md.validateLink(g))return!1;i=c.pos;let h=i,f=s,y=o;for(;i<s;){let T=a.charCodeAt(i);if(T===10){let R=ta(e,o);if(R<0)break;i=R,s=$n(e,o++)}else if(he(T))i++;else break}let v=i,p=e.md.helpers.parseLinkTitle(a,i,s);for(;p.can_continue;){let T=ta(e,o);if(T<0)break;i=T,s=$n(e,o++),p=e.md.helpers.parseLinkTitle(a,i,s,p)}let m;for(v!==h&&p.ok?(m=p.str,i=p.pos):(m="",i=h,s=f,o=y);i<s&&he(a.charCodeAt(i));)i++;if(i<s&&a.charCodeAt(i)!==10&&m)for(m="",i=h,s=f,o=y;i<s&&he(a.charCodeAt(i));)i++;if(i<s&&a.charCodeAt(i)!==10)return!1;let b=ra(l);if(!b)return!1;if(r)return!0;typeof e.env.references>"u"&&(e.env.references={}),typeof e.env.references[b]>"u"&&(e.env.references[b]={title:m,href:g});let x=e.push("reference_definition","",0);x.map=[n,o],x.hidden=!0;let _=Object.create(null);return _.label=b,x.meta=_,e.line=o,!0}var sv=["address","article","aside","base","basefont","blockquote","body","caption","center","col","colgroup","dd","details","dialog","dir","div","dl","dt","fieldset","figcaption","figure","footer","form","frame","frameset","h1","h2","h3","h4","h5","h6","head","header","hr","html","iframe","legend","li","link","main","menu","menuitem","nav","noframes","ol","optgroup","option","p","param","search","section","summary","table","tbody","td","tfoot","th","thead","title","tr","track","ul"],ru=`<[A-Za-z][A-Za-z0-9\\-]*(?:\\s+[a-zA-Z_:][a-zA-Z0-9:._-]*(?:\\s*=\\s*(?:[^"'=<>\`\\x00-\\x20]+|'[^']*'|"[^"]*"))?)*\\s*\\/?>`,au="<\\/[A-Za-z][A-Za-z0-9\\-]*\\s*>";new RegExp(`^(?:${ru}|${au}|<!---?>|<!--(?:[^-]|-[^-]|--[^>])*-->|<[?][\\s\\S]*?[?]>|<![A-Za-z][^>]*>|<!\\[CDATA\\[[\\s\\S]*?\\]\\]>)`);var iu=new RegExp(`^(?:${ru}|${au})`),Ut=[[/^<(script|pre|style|textarea)(?=(\s|>|$))/i,/<\/(script|pre|style|textarea)>/i,!0],[/^<!--/,/-->/,!0],[/^<\?/,/\?>/,!0],[/^<![A-Za-z]/,/>/,!0],[/^<!\[CDATA\[/,/\]\]>/,!0],[new RegExp(`^</?(${sv.join("|")})(?=(\\s|/?>|$))`,"i"),/^$/,!0],[new RegExp(`${iu.source}\\s*$`),/^$/,!1]];function lv(e,n,t,r){let a=e.bMarks[n]+e.tShift[n],i=e.eMarks[n];if(e.sCount[n]-e.blkIndent>=4||!e.md.options.html||e.src.charCodeAt(a)!==60)return!1;let o=e.src.slice(a,i),s=0;for(;s<Ut.length&&!Ut[s][0].test(o);s++);if(s===Ut.length)return!1;if(r)return Ut[s][2];let l=n+1,u=Ut[s][1].test("");if(!Ut[s][1].test(o)){for(;l<t&&!(e.sCount[l]<e.blkIndent&&(u||!e.isEmpty(l)));l++)if(a=e.bMarks[l]+e.tShift[l],i=e.eMarks[l],o=e.src.slice(a,i),Ut[s][1].test(o)){o.length!==0&&l++;break}}e.line=l;let d=e.push("html_block","",0);return d.map=[n,l],d.content=e.getLines(n,l,e.blkIndent,!0),!0}function uv(e,n,t,r){let a=e.bMarks[n]+e.tShift[n],i=e.eMarks[n];if(e.sCount[n]-e.blkIndent>=4)return!1;let o=e.src.charCodeAt(a);if(o!==35||a>=i)return!1;let s=1;for(o=e.src.charCodeAt(++a);o===35&&a<i&&s<=6;)s++,o=e.src.charCodeAt(++a);if(s>6||a<i&&!he(o))return!1;if(r)return!0;i=e.skipSpacesBack(i,a);let l=e.skipCharsBack(i,35,a);l>a&&he(e.src.charCodeAt(l-1))&&(i=l),e.line=n+1;let u=e.push("heading_open",`h${s}`,1);u.markup="########".slice(0,s),u.map=[n,e.line];let d=e.push("inline","",0);d.content=aa(e.src.slice(a,i)),d.map=[n,e.line],d.children=[];let c=e.push("heading_close",`h${s}`,-1);return c.markup="########".slice(0,s),!0}function cv(e,n,t){let r=e.md.block.ruler.getRules("paragraph");if(e.sCount[n]-e.blkIndent>=4)return!1;let a=e.parentType;e.parentType="paragraph";let i=0,o,s=n+1;for(;s<t&&!e.isEmpty(s);s++){if(e.sCount[s]-e.blkIndent>3)continue;if(e.sCount[s]>=e.blkIndent){let h=e.bMarks[s]+e.tShift[s],f=e.eMarks[s];if(h<f&&(o=e.src.charCodeAt(h),(o===45||o===61)&&(h=e.skipChars(h,o),h=e.skipSpaces(h),h>=f))){i=o===61?1:2;break}}if(e.sCount[s]<0)continue;let g=!1;for(let h=0,f=r.length;h<f;h++)if(r[h](e,s,t,!0)){g=!0;break}if(g)break}if(!i)return e.parentType=a,!1;let l=aa(e.getLines(n,s,e.blkIndent,!1));e.line=s+1;let u=e.push("heading_open",`h${i}`,1);u.markup=String.fromCharCode(o),u.map=[n,e.line];let d=e.push("inline","",0);d.content=l,d.map=[n,e.line-1],d.children=[];let c=e.push("heading_close",`h${i}`,-1);return c.markup=String.fromCharCode(o),e.parentType=a,!0}function dv(e,n,t){let r=e.md.block.ruler.getRules("paragraph"),a=e.parentType,i=n+1;for(e.parentType="paragraph";i<t&&!e.isEmpty(i);i++){if(e.sCount[i]-e.blkIndent>3||e.sCount[i]<0)continue;let u=!1;for(let d=0,c=r.length;d<c;d++)if(r[d](e,i,t,!0)){u=!0;break}if(u)break}let o=aa(e.getLines(n,i,e.blkIndent,!1));e.line=i;let s=e.push("paragraph_open","p",1);s.map=[n,e.line];let l=e.push("inline","",0);return l.content=o,l.map=[n,e.line],l.children=[],e.push("paragraph_close","p",-1),e.parentType=a,!0}var na=[["table",Zp,["paragraph","reference"]],["code",Qp],["fence",ev,["paragraph","reference","blockquote","list"]],["blockquote",tv,["paragraph","reference","blockquote","list"]],["hr",nv,["paragraph","reference","blockquote","list"]],["list",av,["paragraph","reference","blockquote"]],["reference",ov],["html_block",lv,["paragraph","reference","blockquote"]],["heading",uv,["paragraph","reference","blockquote"]],["lheading",cv],["paragraph",dv]],ou=class{constructor(){q(this,"ruler",new Kn),q(this,"State",nu);for(let e=0;e<na.length;e++)this.ruler.push(na[e][0],na[e][1],{alt:(na[e][2]||[]).slice()})}tokenize(e,n,t){let r=this.ruler.getRules(""),a=r.length,i=e.md.options.maxNesting,o=n,s=!1;for(;o<t&&(e.line=o=e.skipEmptyLines(o),!(o>=t||e.sCount[o]<e.blkIndent));){if(e.level>=i){e.line=t;break}let l=e.line,u=!1;for(let d=0;d<a;d++)if(u=r[d](e,o,t,!1),u){if(l>=e.line)throw new Error("block rule didn't increment state.line");break}if(!u)throw new Error("none of the block rules matched");e.tight=!s,e.isEmpty(e.line-1)&&(s=!0),o=e.line,o<t&&e.isEmpty(o)&&(s=!0,o++,e.line=o)}}parse(e,n,t,r){if(!e)return;let a=new this.State(e,n,t,r);this.tokenize(a,a.line,a.lineMax)}},su=class{constructor(e,n,t,r){q(this,"pos",0),q(this,"level",0),q(this,"pending",""),q(this,"pendingLevel",0),q(this,"cache",{}),q(this,"backticks",{}),q(this,"backticksScanned",!1),q(this,"skipHtmlSpecial",!1),q(this,"linkLevel",0),q(this,"delimiters",[]),q(this,"_prev_delimiters",[]),q(this,"Token",Ot),this.src=e,this.env=t,this.md=n,this.tokens=r,this.tokens_meta=Array(r.length),this.posMax=this.src.length}pushPending(){let e=new Ot("text","",0);return e.content=this.pending,e.level=this.pendingLevel,this.tokens.push(e),this.pending="",e}push(e,n,t){this.pending&&this.pushPending();let r=new Ot(e,n,t),a;return t<0&&(this.level--,this.delimiters=this._prev_delimiters.pop()),r.level=this.level,t>0&&(this.level++,this._prev_delimiters.push(this.delimiters),this.delimiters=[],a={delimiters:this.delimiters}),this.pendingLevel=this.level,this.tokens.push(r),this.tokens_meta.push(a),r}scanDelims(e,n){let t=this.posMax,r=this.src.charCodeAt(e),a;if(e===0)a=32;else if(e===1)a=this.src.charCodeAt(0),(a&63488)===55296&&(a=65533);else if(a=this.src.charCodeAt(e-1),(a&64512)===56320){let f=this.src.charCodeAt(e-2);a=(f&64512)===55296?65536+(f-55296<<10)+(a-56320):65533}else(a&64512)===55296&&(a=65533);let i=e;for(;i<t&&this.src.charCodeAt(i)===r;)i++;let o=i-e,s=i<t?this.src.charCodeAt(i):32;if((s&64512)===55296){let f=this.src.charCodeAt(i+1);s=(f&64512)===56320?65536+(s-55296<<10)+(f-56320):65533}else(s&64512)===56320&&(s=65533);let l=Yn(a)||Xn(a),u=Yn(s)||Xn(s),d=qn(a),c=qn(s),g=!c&&(!u||d||l),h=!d&&(!l||c||u);return{can_open:g&&(n||!h||l),can_close:h&&(n||!g||u),length:o}}};function hv(e){switch(e){case 10:case 33:case 35:case 36:case 37:case 38:case 42:case 43:case 45:case 58:case 60:case 61:case 62:case 64:case 91:case 92:case 93:case 94:case 95:case 96:case 123:case 125:case 126:return!0;default:return!1}}function fv(e,n){let t=e.pos;for(;t<e.posMax&&!hv(e.src.charCodeAt(t));)t++;return t===e.pos?!1:(n||(e.pending+=e.src.slice(e.pos,t)),e.pos=t,!0)}function gv(e){return e>=65&&e<=90||e>=97&&e<=122}function pv(e){return e>=65&&e<=90||e>=97&&e<=122||e>=48&&e<=57||e===43||e===45||e===46}function vv(e,n){if(!e.md.options.linkify||e.linkLevel>0)return!1;let t=e.pos,r=e.posMax;if(t+3>r||e.src.charCodeAt(t)!==58||e.src.charCodeAt(t+1)!==47||e.src.charCodeAt(t+2)!==47)return!1;let a=t-Math.min(10,e.pending.length,t),i=t;for(;i>a&&pv(e.src.charCodeAt(i-1));)i--;if(i===t||!gv(e.src.charCodeAt(i)))return!1;let o=t-i,s=e.md.linkify.matchAtStart(e.src.slice(i));if(!s)return!1;let l=s.url;if(l.length<=o)return!1;let u=l.length;for(;u>0&&l.charCodeAt(u-1)===42;)u--;u!==l.length&&(l=l.slice(0,u));let d=e.md.normalizeLink(l);if(!e.md.validateLink(d))return!1;if(!n){e.pending=e.pending.slice(0,-o);let c=e.push("link_open","a",1);c.attrs=[["href",d]],c.markup="linkify",c.info="auto";let g=e.push("text","",0);g.content=e.md.normalizeLinkText(l);let h=e.push("link_close","a",-1);h.markup="linkify",h.info="auto"}return e.pos+=l.length-o,!0}function mv(e,n){let t=e.pos;if(e.src.charCodeAt(t)!==10)return!1;let r=e.pending.length-1,a=e.posMax;if(!n)if(r>=0&&e.pending.charCodeAt(r)===32)if(r>=1&&e.pending.charCodeAt(r-1)===32){let i=r-1;for(;i>=1&&e.pending.charCodeAt(i-1)===32;)i--;e.pending=e.pending.slice(0,i),e.push("hardbreak","br",0)}else e.pending=e.pending.slice(0,-1),e.push("softbreak","br",0);else e.push("softbreak","br",0);for(t++;t<a&&he(e.src.charCodeAt(t));)t++;return e.pos=t,!0}var xi=[];for(let e=0;e<256;e++)xi.push(0);"\\!\"#$%&'()*+,./:;<=>?@[]^_`{|}~-".split("").forEach(function(e){xi[e.charCodeAt(0)]=1});function bv(e,n){let t=e.pos,r=e.posMax;if(e.src.charCodeAt(t)!==92||(t++,t>=r))return!1;let a=e.src.charCodeAt(t);if(a===10){for(n||e.push("hardbreak","br",0),t++;t<r&&(a=e.src.charCodeAt(t),!!he(a));)t++;return e.pos=t,!0}if(a===32){if(!n){let s=e.push("text_special","",0);s.content="\\",s.markup="\\",s.info="escape"}return e.pos=t,!0}let i=e.src[t];if(a>=55296&&a<=56319&&t+1<r){let s=e.src.charCodeAt(t+1);s>=56320&&s<=57343&&(i+=e.src[t+1],t++)}let o="\\"+i;if(!n){let s=e.push("text_special","",0);a<256&&xi[a]!==0?s.content=i:s.content=o,s.markup=o,s.info="escape"}return e.pos=t+1,!0}function yv(e){let n={},t=0;for(;(t=e.indexOf("`",t))!==-1;){let r=t;for(;e.charCodeAt(++t)===96;);n[t-r]=r}return n}function _v(e,n){var t;let r=e.pos;if(e.src.charCodeAt(r)!==96)return!1;let a=e.posMax,i=r+1;for(;i<a&&e.src.charCodeAt(i)===96;)i++;let o=e.src.slice(r,i),s=o.length;if(e.backticksScanned||(e.backticks=yv(e.src),e.backticksScanned=!0),((t=e.backticks[s])!==null&&t!==void 0?t:-1)>=i){let l=i,u;for(;(u=e.src.indexOf("`",l))!==-1&&u<a;){for(l=u+1;e.src.charCodeAt(l)===96;)l++;if(l>a)break;if(l-u===s){if(!n){let d=e.push("code_inline","code",0);d.markup=o;let c=e.src.slice(i,u).replace(/\n/g," ");c.startsWith(" ")&&c.endsWith(" ")&&/[^ ]/.test(c)&&(c=c.slice(1,-1)),d.content=c}return e.pos=l,!0}}}return n||(e.pending+=o),e.pos=i,!0}function xv(e,n){let t=e.pos,r=e.src.charCodeAt(t);if(n||r!==126)return!1;let a=e.scanDelims(e.pos,!0),i=a.length,o=String.fromCharCode(r);if(i<2)return!1;let s;i%2&&(s=e.push("text","",0),s.content=o,i--);for(let l=0;l<i;l+=2)s=e.push("text","",0),s.content=o+o,e.delimiters.push({marker:r,length:0,token:e.tokens.length-1,end:-1,open:a.can_open,close:a.can_close});return e.pos+=a.length,!0}function jl(e,n){let t,r=[],a=n.length;for(let i=0;i<a;i++){let o=n[i];if(o.marker!==126||o.end===-1)continue;let s=n[o.end];t=e.tokens[o.token],t.type="s_open",t.tag="s",t.nesting=1,t.markup="~~",t.content="",t=e.tokens[s.token],t.type="s_close",t.tag="s",t.nesting=-1,t.markup="~~",t.content="",e.tokens[s.token-1].type==="text"&&e.tokens[s.token-1].content==="~"&&r.push(s.token-1)}for(;r.length;){let i=r.pop(),o=i+1;for(;o<e.tokens.length&&e.tokens[o].type==="s_close";)o++;o--,i!==o&&(t=e.tokens[o],e.tokens[o]=e.tokens[i],e.tokens[i]=t)}}function Tv(e){let n=e.tokens_meta,t=e.tokens_meta.length;jl(e,e.delimiters);for(let a=0;a<t;a++){var r;let i=(r=n[a])===null||r===void 0?void 0:r.delimiters;i&&jl(e,i)}}var lu={tokenize:xv,postProcess:Tv};function Ev(e,n){let t=e.pos,r=e.src.charCodeAt(t);if(n||r!==95&&r!==42)return!1;let a=e.scanDelims(e.pos,r===42);for(let i=0;i<a.length;i++){let o=e.push("text","",0);o.content=String.fromCharCode(r),e.delimiters.push({marker:r,length:a.length,token:e.tokens.length-1,end:-1,open:a.can_open,close:a.can_close})}return e.pos+=a.length,!0}function $l(e,n){let t=n.length;for(let r=t-1;r>=0;r--){let a=n[r];if(a.marker!==95&&a.marker!==42||a.end===-1)continue;let i=n[a.end],o=r>0&&n[r-1].end===a.end+1&&n[r-1].marker===a.marker&&n[r-1].token===a.token-1&&n[a.end+1].token===i.token+1,s=String.fromCharCode(a.marker),l=e.tokens[a.token];l.type=o?"strong_open":"em_open",l.tag=o?"strong":"em",l.nesting=1,l.markup=o?s+s:s,l.content="";let u=e.tokens[i.token];u.type=o?"strong_close":"em_close",u.tag=o?"strong":"em",u.nesting=-1,u.markup=o?s+s:s,u.content="",o&&(e.tokens[n[r-1].token].content="",e.tokens[n[a.end+1].token].content="",r--)}}function Sv(e){let n=e.tokens_meta,t=e.tokens_meta.length;$l(e,e.delimiters);for(let a=0;a<t;a++){var r;let i=(r=n[a])===null||r===void 0?void 0:r.delimiters;i&&$l(e,i)}}var uu={tokenize:Ev,postProcess:Sv};function Rv(e,n){let t,r,a,i,o="",s="",l=e.pos,u=!0;if(e.src.charCodeAt(e.pos)!==91)return!1;let d=e.pos,c=e.posMax,g=e.pos+1,h=e.md.helpers.parseLinkLabel(e,e.pos,!0);if(h<0)return!1;let f=h+1;if(f<c&&e.src.charCodeAt(f)===40){for(u=!1,f++;f<c&&(t=e.src.charCodeAt(f),!(!he(t)&&t!==10));f++);if(f>=c)return!1;if(l=f,a=e.md.helpers.parseLinkDestination(e.src,f,e.posMax),a.ok){for(o=e.md.normalizeLink(a.str),e.md.validateLink(o)?f=a.pos:o="",l=f;f<c&&(t=e.src.charCodeAt(f),!(!he(t)&&t!==10));f++);if(a=e.md.helpers.parseLinkTitle(e.src,f,e.posMax),f<c&&l!==f&&a.ok)for(s=a.str,f=a.pos;f<c&&(t=e.src.charCodeAt(f),!(!he(t)&&t!==10));f++);}(f>=c||e.src.charCodeAt(f)!==41)&&(u=!0),f++}if(u){if(typeof e.env.references>"u")return!1;if(f<c&&e.src.charCodeAt(f)===91?(l=f+1,f=e.md.helpers.parseLinkLabel(e,f),f>=0?r=e.src.slice(l,f++):f=h+1):f=h+1,r||(r=e.src.slice(g,h)),r=ra(r),i=e.env.references[r],!i)return e.pos=d,!1;o=i.href,s=i.title}if(!n){e.pos=g,e.posMax=h;let y=e.push("link_open","a",1),v=[["href",o]];if(y.attrs=v,s&&v.push(["title",s]),r){let p=Object.create(null);p.label=r,y.meta=p}e.linkLevel++,e.md.inline.tokenize(e),e.linkLevel--,e.push("link_close","a",-1)}return e.pos=f,e.posMax=c,!0}function Av(e,n){let t,r,a,i,o,s,l,u,d="",c=e.pos,g=e.posMax;if(e.src.charCodeAt(e.pos)!==33||e.src.charCodeAt(e.pos+1)!==91)return!1;let h=e.pos+2,f=e.md.helpers.parseLinkLabel(e,e.pos+1,!1);if(f<0)return!1;if(i=f+1,i<g&&e.src.charCodeAt(i)===40){for(i++;i<g&&(t=e.src.charCodeAt(i),!(!he(t)&&t!==10));i++);if(i>=g)return!1;for(u=i,s=e.md.helpers.parseLinkDestination(e.src,i,e.posMax),s.ok&&(d=e.md.normalizeLink(s.str),e.md.validateLink(d)?i=s.pos:d=""),u=i;i<g&&(t=e.src.charCodeAt(i),!(!he(t)&&t!==10));i++);if(s=e.md.helpers.parseLinkTitle(e.src,i,e.posMax),i<g&&u!==i&&s.ok)for(l=s.str,i=s.pos;i<g&&(t=e.src.charCodeAt(i),!(!he(t)&&t!==10));i++);else l="";if(i>=g||e.src.charCodeAt(i)!==41)return e.pos=c,!1;i++}else{if(typeof e.env.references>"u")return!1;if(i<g&&e.src.charCodeAt(i)===91?(u=i+1,i=e.md.helpers.parseLinkLabel(e,i),i>=0?a=e.src.slice(u,i++):i=f+1):i=f+1,a||(a=e.src.slice(h,f)),a=ra(a),o=e.env.references[a],!o)return e.pos=c,!1;d=o.href,l=o.title}if(!n){r=e.src.slice(h,f);let y=[];e.md.inline.parse(r,e.md,e.env,y);let v=e.push("image","img",0),p=[["src",d],["alt",""]];if(v.attrs=p,v.children=y,v.content=r,l&&p.push(["title",l]),a){let m=Object.create(null);m.label=a,v.meta=m}}return e.pos=i,e.posMax=g,!0}var Dv=/^([a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*)$/,wv=/^([a-zA-Z][a-zA-Z0-9+.-]{1,31}):([^<>\x00-\x20]*)$/;function kv(e,n){let t=e.pos;if(e.src.charCodeAt(t)!==60)return!1;let r=e.pos,a=e.posMax;for(;;){if(++t>=a)return!1;let o=e.src.charCodeAt(t);if(o===60)return!1;if(o===62)break}let i=e.src.slice(r+1,t);if(wv.test(i)){let o=e.md.normalizeLink(i);if(!e.md.validateLink(o))return!1;if(!n){let s=e.push("link_open","a",1);s.attrs=[["href",o]],s.markup="autolink",s.info="auto";let l=e.push("text","",0);l.content=e.md.normalizeLinkText(i);let u=e.push("link_close","a",-1);u.markup="autolink",u.info="auto"}return e.pos+=i.length+2,!0}if(Dv.test(i)){let o=e.md.normalizeLink(`mailto:${i}`);if(!e.md.validateLink(o))return!1;if(!n){let s=e.push("link_open","a",1);s.attrs=[["href",o]],s.markup="autolink",s.info="auto";let l=e.push("text","",0);l.content=e.md.normalizeLinkText(i);let u=e.push("link_close","a",-1);u.markup="autolink",u.info="auto"}return e.pos+=i.length+2,!0}return!1}function Cv(e){return/^<a[>\s]/i.test(e)}function Fv(e){return/^<\/a\s*>/i.test(e)}function cu(e){let n=e|32;return n>=97&&n<=122}function Lv(e,n){let t=e.src,r,a;if(t.startsWith("<!--",n)){if(t.startsWith(">",n+4))return n+5;if(t.startsWith("->",n+4))return n+6;r=n+4,a="-->"}else if(t.startsWith("<![CDATA[",n))r=n+9,a="]]>";else if(t.charCodeAt(n+1)===63)r=n+2,a="?>";else if(cu(t.charCodeAt(n+2)))r=n+3,a=">";else return-1;if(e.skipHtmlSpecial)return-1;let i=t.indexOf(a,r);return i<0?(e.skipHtmlSpecial=!0,-1):i+a.length}function Pv(e,n){if(!e.md.options.html)return!1;let t=e.posMax,r=e.pos;if(e.src.charCodeAt(r)!==60||r+2>=t)return!1;let a,i=e.src.charCodeAt(r+1);if(i===33||i===63)a=Lv(e,r);else if(i===47||cu(i)){let o=e.src.slice(r).match(iu);a=o?r+o[0].length:-1}else return!1;if(a<0)return!1;if(!n){let o=e.push("html_inline","",0);o.content=e.src.slice(r,a),Cv(o.content)&&e.linkLevel++,Fv(o.content)&&e.linkLevel--}return e.pos=a,!0}var Iv=/^&#((?:x[a-f0-9]{1,6}|[0-9]{1,7}));/i,Gv=/^&([a-z][a-z0-9]{1,31});/i;function zv(e,n){let t=e.pos,r=e.posMax;if(e.src.charCodeAt(t)!==38||t+1>=r)return!1;if(e.src.charCodeAt(t+1)===35){let a=e.src.slice(t).match(Iv);if(a){if(!n){let i=a[1][0].toLowerCase()==="x"?parseInt(a[1].slice(1),16):parseInt(a[1],10),o=e.push("text_special","",0);o.content=_i(i)?Vn(i):Vn(65533),o.markup=a[0],o.info="entity"}return e.pos+=a[0].length,!0}}else{let a=e.src.slice(t).match(Gv);if(a){let i=Qr(a[0]);if(i!==a[0]){if(!n){let o=e.push("text_special","",0);o.content=i,o.markup=a[0],o.info="entity"}return e.pos+=a[0].length,!0}}}return!1}function Vl(e){let n={},t=e.length;if(!t)return;let r=0,a=-2,i=[];for(let o=0;o<t;o++){let s=e[o];if(i.push(0),(e[r].marker!==s.marker||a!==s.token-1)&&(r=o),a=s.token,s.length=s.length||0,!s.close)continue;n.hasOwnProperty(s.marker)||(n[s.marker]=[-1,-1,-1,-1,-1,-1]);let l=n[s.marker][(s.open?3:0)+s.length%3],u=r-i[r]-1,d=u;for(;u>l;u-=i[u]+1){let c=e[u];if(c.marker===s.marker&&c.open&&c.end<0){let g=!1;if((c.close||s.open)&&(c.length+s.length)%3===0&&(c.length%3!==0||s.length%3!==0)&&(g=!0),!g){let h=u>0&&!e[u-1].open?i[u-1]+1:0;i[o]=o-u+h,i[u]=h,s.open=!1,c.end=o,c.close=!1,d=-1,a=-2;break}}}d!==-1&&(n[s.marker][(s.open?3:0)+(s.length||0)%3]=d)}}function Bv(e){let n=e.tokens_meta,t=e.tokens_meta.length;Vl(e.delimiters);for(let a=0;a<t;a++){var r;let i=(r=n[a])===null||r===void 0?void 0:r.delimiters;i&&Vl(i)}}function Nv(e){let n,t,r=0,a=e.tokens,i=e.tokens.length;for(n=t=0;n<i;n++)a[n].nesting<0&&r--,a[n].level=r,a[n].nesting>0&&r++,a[n].type==="text"&&n+1<i&&a[n+1].type==="text"?a[n+1].content=a[n].content+a[n+1].content:(n!==t&&(a[t]=a[n]),t++);n!==t&&(a.length=t)}var bi=[["text",fv],["linkify",vv],["newline",mv],["escape",bv],["backticks",_v],["strikethrough",lu.tokenize],["emphasis",uu.tokenize],["link",Rv],["image",Av],["autolink",kv],["html_inline",Pv],["entity",zv]],yi=[["balance_pairs",Bv],["strikethrough",lu.postProcess],["emphasis",uu.postProcess],["fragments_join",Nv]],du=class{constructor(){q(this,"ruler",new Kn),q(this,"ruler2",new Kn),q(this,"State",su);for(let e=0;e<bi.length;e++)this.ruler.push(bi[e][0],bi[e][1]);for(let e=0;e<yi.length;e++)this.ruler2.push(yi[e][0],yi[e][1])}skipToken(e){let n=e.pos,t=this.ruler.getRules(""),r=t.length,a=e.md.options.maxNesting,i=e.cache;if(typeof i[n]<"u"){e.pos=i[n];return}let o=!1;if(e.level<a){for(let s=0;s<r;s++)if(e.level++,o=t[s](e,!0),e.level--,o){if(n>=e.pos)throw new Error("inline rule didn't increment state.pos");break}}else e.pos=e.posMax;o||e.pos++,i[n]=e.pos}tokenize(e){let n=this.ruler.getRules(""),t=n.length,r=e.posMax,a=e.md.options.maxNesting;for(;e.pos<r;){let i=e.pos,o=!1;if(e.level<a){for(let s=0;s<t;s++)if(o=n[s](e,!1),o){if(i>=e.pos)throw new Error("inline rule didn't increment state.pos");break}}if(o){if(e.pos>=r)break;continue}e.pending+=e.src[e.pos++]}e.pending&&e.pushPending()}parse(e,n,t,r){let a=new this.State(e,n,t,r);this.tokenize(a);let i=this.ruler2.getRules(""),o=i.length;for(let s=0;s<o;s++)i[s](a)}},Mv={default:{options:{html:!1,xhtmlOut:!1,breaks:!1,langPrefix:"language-",linkify:!1,typographer:!1,quotes:"\u201C\u201D\u2018\u2019",highlight:null,maxNesting:100},components:{core:{},block:{},inline:{}}},zero:{options:{html:!1,xhtmlOut:!1,breaks:!1,langPrefix:"language-",linkify:!1,typographer:!1,quotes:"\u201C\u201D\u2018\u2019",highlight:null,maxNesting:20},components:{core:{rules:["normalize","block","strip_references","inline","text_join"]},block:{rules:["paragraph"]},inline:{rules:["text"],rules2:["balance_pairs","fragments_join"]}}},commonmark:{options:{html:!0,xhtmlOut:!0,breaks:!1,langPrefix:"language-",linkify:!1,typographer:!1,quotes:"\u201C\u201D\u2018\u2019",highlight:null,maxNesting:20},components:{core:{rules:["normalize","block","strip_references","inline","text_join"]},block:{rules:["blockquote","code","fence","heading","hr","html_block","lheading","list","reference","paragraph"]},inline:{rules:["autolink","backticks","emphasis","entity","escape","html_inline","image","link","newline","text"],rules2:["balance_pairs","emphasis","fragments_join"]}}}},Wv=/^(vbscript|javascript|file|data):/,Uv=/^data:image\/(gif|png|jpeg|webp);/,ql=["http:","https:","mailto:"],at=class{validateLink(e){let n=e.trim().toLowerCase();return Wv.test(n)?Uv.test(n):!0}normalizeLink(e){let n=Hn(e,!0);if(n.hostname&&(!n.protocol||ql.indexOf(n.protocol)>=0))try{n.hostname=gi.toASCII(n.hostname)}catch{}return n.auth&&(n.auth=Et(n.auth)),n.hostname&&(n.hostname=Et(n.hostname)),n.pathname&&(n.pathname=Et(n.pathname)),n.search&&(n.search=Et(n.search)),n.hash&&(n.hash=Et(n.hash)),mn(n)}normalizeLinkText(e){let n=Hn(e,!0);if(n.hostname&&(!n.protocol||ql.indexOf(n.protocol)>=0))try{n.hostname=gi.toUnicode(n.hostname)}catch{}return On(mn(n),On.defaultChars+"%")}constructor(...e){q(this,"inline",new du),q(this,"block",new ou),q(this,"core",new tu),q(this,"renderer",new Zl),q(this,"linkify",new kl),q(this,"utils",up),q(this,"helpers",Object.assign({},Rp));let[n,t]=e;typeof n=="string"?(this.configure(n),t&&this.set(t)):(this.configure("default"),this.set(n||{}))}set(e){return Object.assign(this.options,e),this}configure(e){let n;if(typeof e=="string"){let a=e;if(n=Mv[a],!n)throw new Error(`Wrong 'markdown-it' preset "${a}", check name`)}else n=e;if(!n)throw new Error("Wrong `markdown-it` preset, can't be empty");n.options&&(this.options={...n.options});let t=n.components;if(t){var r;["core","block","inline"].forEach(i=>{var o;let s=(o=t[i])===null||o===void 0?void 0:o.rules;s&&this[i].ruler.enableOnly(s)});let a=(r=t.inline)===null||r===void 0?void 0:r.rules2;a&&this.inline.ruler2.enableOnly(a)}return this}enable(e,n=!1){let t=[];Array.isArray(e)||(e=[e]),["core","block","inline"].forEach(a=>{t=t.concat(this[a].ruler.enable(e,!0))}),t=t.concat(this.inline.ruler2.enable(e,!0));let r=e.filter(a=>t.indexOf(a)<0);if(r.length&&!n)throw new Error(`MarkdownIt. Failed to enable unknown rule(s): ${r}`);return this}disable(e,n=!1){let t=[];Array.isArray(e)||(e=[e]),["core","block","inline"].forEach(a=>{t=t.concat(this[a].ruler.disable(e,!0))}),t=t.concat(this.inline.ruler2.disable(e,!0));let r=e.filter(a=>t.indexOf(a)<0);if(r.length&&!n)throw new Error(`MarkdownIt. Failed to disable unknown rule(s): ${r}`);return this}use(e,...n){return e.apply(e,[this,...n]),this}parse(e,n){if(typeof e!="string")throw new Error("Input data should be a String");let t=new this.core.State(e,this,n);return this.core.process(t),t.tokens}render(e,n={}){return this.renderer.render(this.parse(e,n),this.options,n)}parseInline(e,n){let t=new this.core.State(e,this,n);return t.inlineMode=!0,this.core.process(t),t.tokens}renderInline(e,n={}){return this.renderer.render(this.parseInline(e,n),this.options,n)}};q(at,"Token",Ot);q(at,"Ruler",Kn);q(at,"Renderer",Zl);q(at,"ParserCore",tu);q(at,"StateCore",Ql);q(at,"ParserBlock",ou);q(at,"StateBlock",nu);q(at,"ParserInline",du);q(at,"StateInline",su);var hu=Yl(at);window.BrainSigma={Graph:de,Sigma:Ws,forceAtlas2:fu.default,FA2Layout:gu.default,circular:pu.default,MarkdownIt:hu};})();
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
/*! graphology-layout 0.6.1 (MIT)

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
/*! graphology-layout-forceatlas2 0.10.1 (MIT)

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
