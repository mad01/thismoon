/* Smash: a brick-breaker with a twist — the bricks spell the blocked hostname in the
   kit's 3x5 pixel font. Smash the site you tried to visit. */
(function () {
  "use strict";
  if (!window.ARCADE) return;

  window.ARCADE.register("smash", function (K) {
    var ROW_COLORS = ["#ff6b6b", "#ffa94d", "#ffd43b", "#69db7c", "#66d9e8"];
    var WALL_TOP = K.TOP + 20;

    var bricks, brickSize, totalBricks;
    var paddle, ball, lives, score, wave, phase, flashUntil, tick;

    function buildBricks() {
      bricks = [];
      var cells = K.host.length * 4 - 1;
      brickSize = Math.max(3, Math.min(7, Math.floor((K.W - 16) / cells)));
      var x0 = Math.floor((K.W - cells * brickSize) / 2);
      for (var i = 0; i < K.host.length; i++) {
        var g = K.glyph(K.host[i]);
        for (var r = 0; r < 5; r++) {
          for (var c = 0; c < 3; c++) {
            if (g[r][c] !== "#") continue;
            bricks.push({
              x: x0 + (i * 4 + c) * brickSize,
              y: WALL_TOP + r * brickSize,
              color: ROW_COLORS[r]
            });
          }
        }
      }
      totalBricks = bricks.length;
    }

    function baseSpeed() {
      return 1.5 + (wave - 1) * 0.25; // gentle start, +0.25 per cleared wall
    }
    function speed() {
      var progress = 1 - bricks.length / totalBricks;
      return Math.min(3.4, baseSpeed() + progress * 1.1);
    }

    function serve() {
      phase = "ready"; // ball parked on the paddle until launched
      ball = { x: paddle.x + paddle.w / 2 - 1, y: paddle.y - 4, vx: 0, vy: 0, s: 3 };
    }

    function launch() {
      phase = "run";
      var v = speed();
      var a = Math.random() * 0.6 - 0.3 - Math.PI / 2; // mostly straight up
      ball.vx = Math.cos(a) * v;
      ball.vy = Math.sin(a) * v;
      K.beep(440, 0.06);
    }

    function bounceOffPaddle() {
      var rel = (ball.x + ball.s / 2 - (paddle.x + paddle.w / 2)) / (paddle.w / 2);
      rel = Math.max(-1, Math.min(1, rel));
      var a = rel * 1.05 - Math.PI / 2; // up to ~60 degrees off vertical
      var v = speed();
      ball.vx = Math.cos(a) * v;
      ball.vy = Math.sin(a) * v;
      ball.y = paddle.y - ball.s;
      K.beep(220, 0.05);
    }

    function stepBall() {
      // sub-steps so a fast ball can't tunnel through 3px bricks
      var steps = Math.ceil(speed() / 1.5);
      for (var n = 0; n < steps && phase === "run"; n++) {
        ball.x += ball.vx / steps;
        ball.y += ball.vy / steps;

        if (ball.x < 2) {
          ball.x = 2;
          ball.vx = Math.abs(ball.vx);
          K.beep(160, 0.04);
        } else if (ball.x + ball.s > K.W - 2) {
          ball.x = K.W - 2 - ball.s;
          ball.vx = -Math.abs(ball.vx);
          K.beep(160, 0.04);
        }
        if (ball.y < K.TOP) {
          ball.y = K.TOP;
          ball.vy = Math.abs(ball.vy);
          K.beep(160, 0.04);
        }

        if (
          ball.vy > 0 &&
          ball.y + ball.s >= paddle.y &&
          ball.y + ball.s <= paddle.y + paddle.h + 3 &&
          ball.x + ball.s > paddle.x &&
          ball.x < paddle.x + paddle.w
        ) {
          bounceOffPaddle();
        }

        for (var i = bricks.length - 1; i >= 0; i--) {
          var b = bricks[i];
          if (
            ball.x < b.x + brickSize &&
            ball.x + ball.s > b.x &&
            ball.y < b.y + brickSize &&
            ball.y + ball.s > b.y
          ) {
            bricks.splice(i, 1);
            score += 10;
            K.burst(b.x + brickSize / 2, b.y + brickSize / 2, b.color, 4);
            K.beep(700 + Math.random() * 300, 0.05);
            // flip the axis with the smaller penetration
            var overX = Math.min(ball.x + ball.s - b.x, b.x + brickSize - ball.x);
            var overY = Math.min(ball.y + ball.s - b.y, b.y + brickSize - ball.y);
            if (overX < overY) ball.vx = -ball.vx;
            else ball.vy = -ball.vy;
            break;
          }
        }

        if (bricks.length === 0) {
          score += 100;
          wave += 1;
          flashUntil = tick + 90;
          buildBricks();
          serve();
          K.beep(880, 0.12);
          K.beep(1320, 0.2);
          return;
        }

        if (ball.y > K.H) {
          lives -= 1;
          K.beep(110, 0.25, 0.06);
          if (lives <= 0) K.end();
          else serve();
          return;
        }
      }
    }

    function movePaddle() {
      if (K.keys.left) paddle.x -= 4;
      if (K.keys.right) paddle.x += 4;
      clampPaddle();
    }
    function clampPaddle() {
      paddle.x = Math.max(4, Math.min(K.W - 4 - paddle.w, paddle.x));
      if (phase === "ready") ball.x = paddle.x + paddle.w / 2 - 1;
    }

    return {
      hint: "arrows or mouse - space to launch",
      reset: function () {
        score = 0;
        lives = 3;
        wave = 1;
        tick = 0;
        flashUntil = 0;
        paddle = { x: K.W / 2 - 22, y: K.H - 16, w: 44, h: 5 };
        buildBricks();
        serve();
      },
      update: function () {
        tick += 1;
        movePaddle();
        if (phase === "run") stepBall();
      },
      key: function (code) {
        if ((code === "Space" || code === "ArrowUp") && phase === "ready") launch();
      },
      pointermove: function (x) {
        paddle.x = x - paddle.w / 2;
        clampPaddle();
      },
      pointerdown: function () {
        if (phase === "ready") launch();
      },
      draw: function () {
        var ctx = K.ctx;
        for (var li = 0; li < 3; li++) K.heart(6 + li * 10, 6, li < lives);

        for (var bi = 0; bi < bricks.length; bi++) {
          var b = bricks[bi];
          ctx.fillStyle = b.color;
          ctx.fillRect(b.x, b.y, brickSize, brickSize);
        }

        ctx.fillStyle = "#e6e8eb";
        ctx.fillRect(paddle.x, paddle.y, paddle.w, paddle.h);
        ctx.fillStyle = "#66d9e8";
        ctx.fillRect(paddle.x, paddle.y, 4, paddle.h);
        ctx.fillRect(paddle.x + paddle.w - 4, paddle.y, 4, paddle.h);

        ctx.fillStyle = "#ffffff";
        ctx.fillRect(Math.floor(ball.x), Math.floor(ball.y), ball.s, ball.s);

        if (phase === "ready") {
          if (tick < flashUntil) K.centerText("wave " + wave, 110, 3, "#ffd43b");
          if (tick % 60 < 40) K.centerText("space to launch", 140, 2, "#9aa2ad");
        }
      },
      score: function () {
        return score;
      }
    };
  });
})();
