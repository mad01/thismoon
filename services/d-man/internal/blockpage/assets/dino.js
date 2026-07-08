/* Self-contained endless runner served on d-man-blocked hosts. No dependencies,
   no CDN — a small Chrome-dino homage in a <canvas>. */
(function () {
  "use strict";

  var canvas = document.getElementById("game");
  if (!canvas) return;
  var ctx = canvas.getContext("2d");
  var W = canvas.width;
  var H = canvas.height;
  var GROUND = H - 28; // y of the ground line; the dino's feet rest here
  var GRAVITY = 0.6;
  var JUMP_V = -11;
  var STAND_H = 46;
  var DUCK_H = 26;
  var DINO_W = 40;

  var dino, obstacles, speed, score, best = 0, spawnIn, state; // state: "run" | "over"

  function reset() {
    dino = { x: 56, w: DINO_W, fy: GROUND, vy: 0, ducking: false };
    obstacles = [];
    speed = 6;
    score = 0;
    spawnIn = 40;
    state = "run";
  }

  function dinoHeight() {
    return dino.ducking && dino.fy >= GROUND ? DUCK_H : STAND_H;
  }
  function dinoBox() {
    var h = dinoHeight();
    return { x: dino.x, y: dino.fy - h, w: dino.w, h: h };
  }

  function spawn() {
    if (Math.random() < 0.22) {
      // bird: duck under it, or clear it with a well-timed jump
      var y = GROUND - (Math.random() < 0.5 ? 34 : 62);
      obstacles.push({ x: W + 20, y: y, w: 36, h: 22, bird: true, t: 0 });
    } else {
      var h = 28 + Math.floor(Math.random() * 34);
      var w = 14 + Math.floor(Math.random() * 24);
      obstacles.push({ x: W + 20, y: GROUND - h, w: w, h: h, bird: false });
    }
  }

  function jump() {
    if (state === "over") { reset(); return; }
    if (dino.fy >= GROUND) dino.vy = JUMP_V;
  }

  function hit(a, b) {
    return a.x < b.x + b.w && a.x + a.w > b.x && a.y < b.y + b.h && a.y + a.h > b.y;
  }

  function update() {
    if (state !== "run") return;

    dino.vy += GRAVITY;
    dino.fy += dino.vy;
    if (dino.fy > GROUND) { dino.fy = GROUND; dino.vy = 0; }

    spawnIn -= 1;
    if (spawnIn <= 0) {
      spawn();
      spawnIn = Math.max(28, 90 - speed * 2) + Math.floor(Math.random() * 40);
    }

    var box = dinoBox();
    var pad = 4; // shave the hitbox so grazes feel fair
    var probe = { x: box.x + pad, y: box.y + pad, w: box.w - 2 * pad, h: box.h - 2 * pad };
    for (var i = obstacles.length - 1; i >= 0; i--) {
      var o = obstacles[i];
      o.x -= speed;
      if (o.bird) o.t += 1;
      if (o.x + o.w < 0) { obstacles.splice(i, 1); continue; }
      if (hit(probe, o)) {
        state = "over";
        if (score > best) best = score;
      }
    }

    score += 1;
    speed = 6 + score / 900;
  }

  function drawDino() {
    var h = dinoHeight();
    var x = dino.x, y = dino.fy - h;
    ctx.fillStyle = "#7ee787";
    ctx.fillRect(x, y, dino.w, h);
    ctx.fillStyle = "#0f1115";
    ctx.fillRect(x + dino.w - 12, y + 8, 5, 5); // eye
    if (dino.fy >= GROUND && Math.floor(score / 6) % 2 === 0) {
      ctx.fillRect(x + 6, dino.fy - 4, 8, 4); // running-leg flicker
    }
  }

  function drawObstacle(o) {
    if (o.bird) {
      ctx.fillStyle = "#f0883e";
      ctx.fillRect(o.x, o.y + 8, o.w, 6);
      var wingUp = Math.floor(o.t / 8) % 2 === 0;
      ctx.fillRect(o.x + 12, wingUp ? o.y : o.y + 14, 12, 8);
    } else {
      ctx.fillStyle = "#3fb950";
      ctx.fillRect(o.x, o.y, o.w, o.h);
      ctx.fillRect(o.x - 5, o.y + o.h * 0.4, 5, 6);
      ctx.fillRect(o.x + o.w, o.y + o.h * 0.25, 5, 6);
    }
  }

  function draw() {
    ctx.clearRect(0, 0, W, H);
    ctx.strokeStyle = "#39414d";
    ctx.lineWidth = 2;
    ctx.beginPath();
    ctx.moveTo(0, GROUND + 1);
    ctx.lineTo(W, GROUND + 1);
    ctx.stroke();

    obstacles.forEach(drawObstacle);
    drawDino();

    ctx.fillStyle = "#9aa2ad";
    ctx.font = "600 14px ui-monospace, Menlo, monospace";
    ctx.textAlign = "right";
    ctx.fillText("score " + Math.floor(score / 5) + "   best " + Math.floor(best / 5), W - 12, 22);

    if (state === "over") {
      ctx.fillStyle = "rgba(15,17,21,0.72)";
      ctx.fillRect(0, 0, W, H);
      ctx.fillStyle = "#e6e8eb";
      ctx.textAlign = "center";
      ctx.font = "700 22px system-ui, sans-serif";
      ctx.fillText("game over", W / 2, H / 2 - 6);
      ctx.fillStyle = "#9aa2ad";
      ctx.font = "500 13px system-ui, sans-serif";
      ctx.fillText("space / tap to run again", W / 2, H / 2 + 18);
    }
  }

  function loop() {
    update();
    draw();
    requestAnimationFrame(loop);
  }

  window.addEventListener("keydown", function (e) {
    if (e.code === "Space" || e.code === "ArrowUp") { e.preventDefault(); jump(); }
    else if (e.code === "ArrowDown") { dino.ducking = true; }
  });
  window.addEventListener("keyup", function (e) {
    if (e.code === "ArrowDown") dino.ducking = false;
  });
  canvas.addEventListener("pointerdown", function (e) { e.preventDefault(); jump(); });

  reset();
  loop();
})();
