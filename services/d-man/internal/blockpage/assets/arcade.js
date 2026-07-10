/* Shared arcade kit for the d-man block page. No dependencies, no CDN.
   Owns the 320x240 canvas (CSS upscales it, image-rendering: pixelated),
   a 3x5 pixel font, WebAudio bleeps, particles, input, and the
   title -> play -> game-over state machine. Each game file registers a
   factory via ARCADE.register(name, factory); one game is picked at
   random per page load, and N switches to another. */
window.ARCADE = (function () {
  "use strict";

  var canvas = document.getElementById("game");
  if (!canvas) return null;
  var ctx = canvas.getContext("2d");
  var W = canvas.width; // 320
  var H = canvas.height; // 240
  var TOP = 22; // the strip above this line belongs to the kit HUD

  /* --- 3x5 pixel font: lowercase keys, arcade-uppercase shapes --- */
  var FONT = {
    a: [".#.", "#.#", "###", "#.#", "#.#"],
    b: ["##.", "#.#", "##.", "#.#", "##."],
    c: [".##", "#..", "#..", "#..", ".##"],
    d: ["##.", "#.#", "#.#", "#.#", "##."],
    e: ["###", "#..", "##.", "#..", "###"],
    f: ["###", "#..", "##.", "#..", "#.."],
    g: [".##", "#..", "#.#", "#.#", ".##"],
    h: ["#.#", "#.#", "###", "#.#", "#.#"],
    i: ["###", ".#.", ".#.", ".#.", "###"],
    j: ["..#", "..#", "..#", "#.#", ".#."],
    k: ["#.#", "#.#", "##.", "#.#", "#.#"],
    l: ["#..", "#..", "#..", "#..", "###"],
    m: ["#.#", "###", "###", "#.#", "#.#"],
    n: ["##.", "#.#", "#.#", "#.#", "#.#"],
    o: ["###", "#.#", "#.#", "#.#", "###"],
    p: ["##.", "#.#", "##.", "#..", "#.."],
    q: ["###", "#.#", "#.#", "###", "..#"],
    r: ["##.", "#.#", "##.", "#.#", "#.#"],
    s: [".##", "#..", ".#.", "..#", "##."],
    t: ["###", ".#.", ".#.", ".#.", ".#."],
    u: ["#.#", "#.#", "#.#", "#.#", "###"],
    v: ["#.#", "#.#", "#.#", "#.#", ".#."],
    w: ["#.#", "#.#", "###", "###", "#.#"],
    x: ["#.#", "#.#", ".#.", "#.#", "#.#"],
    y: ["#.#", "#.#", ".#.", ".#.", ".#."],
    z: ["###", "..#", ".#.", "#..", "###"],
    0: ["###", "#.#", "#.#", "#.#", "###"],
    1: [".#.", "##.", ".#.", ".#.", "###"],
    2: ["##.", "..#", ".#.", "#..", "###"],
    3: ["###", "..#", ".##", "..#", "###"],
    4: ["#.#", "#.#", "###", "..#", "..#"],
    5: ["###", "#..", "##.", "..#", "##."],
    6: [".##", "#..", "###", "#.#", "###"],
    7: ["###", "..#", ".#.", ".#.", ".#."],
    8: ["###", "#.#", "###", "#.#", "###"],
    9: ["###", "#.#", "###", "..#", "##."],
    ".": ["...", "...", "...", "...", ".#."],
    "-": ["...", "...", "###", "...", "..."]
  };

  function drawText(str, x, y, scale, color) {
    ctx.fillStyle = color;
    str = String(str).toLowerCase();
    for (var i = 0; i < str.length; i++) {
      var g = FONT[str[i]];
      if (!g) continue; // space and unknown chars render as gaps
      for (var r = 0; r < 5; r++) {
        for (var c = 0; c < 3; c++) {
          if (g[r][c] === "#") {
            ctx.fillRect(x + (i * 4 + c) * scale, y + r * scale, scale, scale);
          }
        }
      }
    }
  }
  function textWidth(str, scale) {
    return (str.length * 4 - 1) * scale;
  }
  function centerText(str, y, scale, color) {
    drawText(str, Math.floor((W - textWidth(str, scale)) / 2), y, scale, color);
  }
  function glyph(ch) {
    return FONT[ch];
  }

  /* --- the blocked hostname, filtered to chars the font can draw --- */
  var host = (location.hostname || "blocked").toLowerCase().replace(/^www\./, "");
  var kept = "";
  for (var hi = 0; hi < host.length; hi++) {
    if (FONT[host[hi]]) kept += host[hi];
  }
  host = kept || "blocked";
  if (host.length > 25) host = host.slice(0, 25);

  /* --- chiptune bleeps (context created lazily, on a user gesture) --- */
  var muted = false;
  var actx = null;
  function beep(freq, dur, vol) {
    if (muted) return;
    try {
      actx = actx || new (window.AudioContext || window.webkitAudioContext)();
      var o = actx.createOscillator();
      var g = actx.createGain();
      o.type = "square";
      o.frequency.value = freq;
      g.gain.setValueAtTime(vol || 0.04, actx.currentTime);
      g.gain.exponentialRampToValueAtTime(0.0001, actx.currentTime + dur);
      o.connect(g);
      g.connect(actx.destination);
      o.start();
      o.stop(actx.currentTime + dur);
    } catch (e) {
      muted = true;
    }
  }

  /* --- particles --- */
  var particles = [];
  function burst(x, y, color, n) {
    for (var i = 0; i < (n || 5); i++) {
      particles.push({
        x: x,
        y: y,
        vx: (Math.random() - 0.5) * 2.4,
        vy: -Math.random() * 2 - 0.3,
        life: 20 + Math.random() * 12,
        color: color
      });
    }
  }
  function stepAndDrawParticles() {
    for (var i = particles.length - 1; i >= 0; i--) {
      var p = particles[i];
      p.x += p.vx;
      p.y += p.vy;
      p.vy += 0.1;
      p.life -= 1;
      if (p.life <= 0) {
        particles.splice(i, 1);
        continue;
      }
      ctx.fillStyle = p.color;
      ctx.fillRect(Math.floor(p.x), Math.floor(p.y), 2, 2);
    }
  }

  function heart(x, y, on) {
    ctx.fillStyle = on ? "#ff6b6b" : "#333b45";
    ctx.fillRect(x + 1, y, 2, 1);
    ctx.fillRect(x + 4, y, 2, 1);
    ctx.fillRect(x, y + 1, 7, 2);
    ctx.fillRect(x + 1, y + 3, 5, 1);
    ctx.fillRect(x + 2, y + 4, 3, 1);
    ctx.fillRect(x + 3, y + 5, 1, 1);
  }

  /* --- state machine + registry --- */
  var keys = { left: false, right: false, up: false, down: false };
  var factories = {};
  var names = [];
  var game = null;
  var gameName = "";
  var state = "title"; // "title" | "play" | "over"
  var tick = 0;
  var best = 0;
  var finalScore = 0;
  var newBest = false;

  function register(name, factory) {
    factories[name] = factory;
    names.push(name);
  }

  function loadBest(name) {
    try {
      return parseInt(localStorage.getItem("dman-arcade-" + name), 10) || 0;
    } catch (e) {
      return 0;
    }
  }
  function saveBest(name, v) {
    try {
      localStorage.setItem("dman-arcade-" + name, String(v));
    } catch (e) {}
  }

  function randomName(exclude) {
    var pool = [];
    for (var i = 0; i < names.length; i++) {
      if (names[i] !== exclude) pool.push(names[i]);
    }
    if (!pool.length) pool = names;
    return pool[Math.floor(Math.random() * pool.length)];
  }

  function switchTo(name) {
    gameName = name;
    game = factories[name](api);
    best = loadBest(name);
    particles.length = 0;
    state = "title";
  }

  function start() {
    game.reset();
    state = "play";
    beep(660, 0.08);
  }

  function end() {
    finalScore = game.score();
    newBest = finalScore > 0 && finalScore > best;
    if (newBest) {
      best = finalScore;
      saveBest(gameName, best);
    }
    state = "over";
    beep(130, 0.3, 0.05);
  }

  /* --- input --- */
  window.addEventListener("keydown", function (e) {
    var c = e.code;
    if (c === "ArrowLeft" || c === "KeyA") keys.left = true;
    if (c === "ArrowRight" || c === "KeyD") keys.right = true;
    if (c === "ArrowUp" || c === "KeyW") keys.up = true;
    if (c === "ArrowDown" || c === "KeyS") keys.down = true;
    if (c.indexOf("Arrow") === 0 || c === "Space") e.preventDefault();
    if (c === "KeyM") {
      muted = !muted;
      return;
    }
    if (c === "KeyN") {
      switchTo(randomName(gameName));
      return;
    }
    if (state === "title") {
      if (c === "Space" || c === "Enter") start();
    } else if (state === "over") {
      if (c === "Space" || c === "Enter") state = "title";
    } else if (game && game.key) {
      game.key(c);
    }
  });
  window.addEventListener("keyup", function (e) {
    var c = e.code;
    if (c === "ArrowLeft" || c === "KeyA") keys.left = false;
    if (c === "ArrowRight" || c === "KeyD") keys.right = false;
    if (c === "ArrowUp" || c === "KeyW") keys.up = false;
    if (c === "ArrowDown" || c === "KeyS") keys.down = false;
  });

  function pointerPos(e) {
    var rect = canvas.getBoundingClientRect();
    return {
      x: ((e.clientX - rect.left) * W) / rect.width,
      y: ((e.clientY - rect.top) * H) / rect.height
    };
  }
  canvas.addEventListener("pointermove", function (e) {
    if (state === "play" && game && game.pointermove) {
      var p = pointerPos(e);
      game.pointermove(p.x, p.y);
    }
  });
  canvas.addEventListener("pointerdown", function (e) {
    e.preventDefault();
    if (state === "title") start();
    else if (state === "over") state = "title";
    else if (game && game.pointerdown) {
      var p = pointerPos(e);
      game.pointerdown(p.x, p.y);
    }
  });

  /* --- background stars --- */
  var stars = [];
  for (var si = 0; si < 40; si++) {
    stars.push({
      x: Math.floor(Math.random() * W),
      y: Math.floor(Math.random() * (H - TOP - 30)) + TOP + 4,
      phase: Math.floor(Math.random() * 120)
    });
  }

  /* --- frame --- */
  function drawFrame() {
    ctx.fillStyle = "#0d1017";
    ctx.fillRect(0, 0, W, H);
    for (var i = 0; i < stars.length; i++) {
      var st = stars[i];
      if ((tick + st.phase) % 120 < 90) {
        ctx.fillStyle = "#2a3140";
        ctx.fillRect(st.x, st.y, 1, 1);
      }
    }

    if (state === "title") {
      centerText(gameName, 66, 5, "#ffd43b");
      centerText(host + " is blocked", 108, 1, "#6b7480");
      if (tick % 60 < 40) centerText("press space", 128, 2, "#e6e8eb");
      if (game.hint) centerText(game.hint, 160, 1, "#6b7480");
      centerText("n next game - m mute", 176, 1, "#3d4552");
      return;
    }

    game.draw(tick);
    stepAndDrawParticles();
    var hud = "score " + game.score() + " best " + best;
    drawText(hud, W - textWidth(hud, 2) - 5, 5, 2, "#9aa2ad");

    if (state === "over") {
      ctx.fillStyle = "rgba(13,16,23,0.8)";
      ctx.fillRect(0, 0, W, H);
      centerText("game over", 84, 4, "#ff6b6b");
      centerText("score " + finalScore, 120, 2, "#e6e8eb");
      if (newBest) centerText("new best", 138, 2, "#ffd43b");
      if (tick % 60 < 40) centerText("press space", 160, 2, "#9aa2ad");
    }
  }

  function loop() {
    tick += 1;
    if (state === "play") game.update(tick);
    drawFrame();
    requestAnimationFrame(loop);
  }

  function boot() {
    if (!names.length) return;
    switchTo(randomName(""));
    loop();
  }
  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", boot);
  } else {
    setTimeout(boot, 0);
  }

  var api = {
    W: W,
    H: H,
    TOP: TOP,
    ctx: ctx,
    keys: keys,
    host: host,
    glyph: glyph,
    drawText: drawText,
    textWidth: textWidth,
    centerText: centerText,
    beep: beep,
    burst: burst,
    heart: heart,
    end: end,
    register: register
  };
  return api;
})();
