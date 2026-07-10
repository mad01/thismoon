/* Gate: an original d-man-themed lane defense. Request packets fall down
   four lanes toward the ports. Good packets (green) must land; bad ones
   (red trackers) must be zapped before they do. Zapping a good packet is
   friendly fire. You are the daemon at the gate. */
(function () {
  "use strict";
  if (!window.ARCADE) return;

  window.ARCADE.register("gate", function (K) {
    var LANES = 4;
    var LANE_W = 60;
    var X0 = (K.W - LANES * LANE_W) / 2;
    var PORT_Y = K.H - 16; // packets land here
    var PLAYER_Y = K.H - 36;
    var PORTS = ["80", "443", "53", "22"]; // flavor labels on the sockets
    var ZAP_COOLDOWN = 10;

    var lane, packets, score, lives, wave, handled, spawnIn, cool, zapFx, portFx, flashUntil, tick;

    function laneCenter(i) {
      return X0 + LANE_W / 2 + i * LANE_W;
    }

    function fallSpeed() {
      return Math.min(2.4, 0.55 + (wave - 1) * 0.18);
    }
    function spawnInterval() {
      return Math.max(24, 64 - (wave - 1) * 6);
    }

    function spawn() {
      packets.push({
        lane: Math.floor(Math.random() * LANES),
        y: K.TOP + 2,
        bad: Math.random() < 0.42,
        wob: Math.floor(Math.random() * 60) // wobble phase, purely cosmetic
      });
    }

    function packetHandled() {
      handled += 1;
      if (handled % 25 === 0) {
        wave += 1;
        flashUntil = tick + 80;
        K.beep(880, 0.12);
        K.beep(1320, 0.2);
      }
    }

    function loseLife() {
      lives -= 1;
      K.beep(110, 0.3, 0.06);
      if (lives <= 0) K.end();
    }

    function zap() {
      if (cool > 0) return;
      cool = ZAP_COOLDOWN;
      zapFx = { lane: lane, t: 6 };
      // the beam hits the packet closest to the ports in this lane
      var target = -1;
      for (var i = 0; i < packets.length; i++) {
        if (packets[i].lane === lane && (target < 0 || packets[i].y > packets[target].y)) {
          target = i;
        }
      }
      if (target < 0) {
        K.beep(300, 0.04, 0.02); // whiff
        return;
      }
      var p = packets[target];
      packets.splice(target, 1);
      packetHandled();
      if (p.bad) {
        score += 15;
        K.burst(laneCenter(p.lane), p.y, "#ff6b6b", 8);
        K.beep(900 + Math.random() * 200, 0.06);
      } else {
        // friendly fire: you just zapped a legitimate request
        K.burst(laneCenter(p.lane), p.y, "#69db7c", 8);
        loseLife();
      }
    }

    function moveTo(l) {
      lane = Math.max(0, Math.min(LANES - 1, l));
    }

    return {
      hint: "arrows pick lane - space zaps bad packets",
      reset: function () {
        lane = 1;
        packets = [];
        score = 0;
        lives = 3;
        wave = 1;
        handled = 0;
        spawnIn = 50;
        cool = 0;
        zapFx = null;
        portFx = [0, 0, 0, 0]; // per-lane landing flash: +good / -bad, counts down
        flashUntil = 0;
        tick = 0;
      },
      update: function () {
        tick += 1;
        if (cool > 0) cool -= 1;
        if (zapFx && --zapFx.t <= 0) zapFx = null;
        for (var f = 0; f < LANES; f++) {
          if (portFx[f] > 0) portFx[f] -= 1;
          if (portFx[f] < 0) portFx[f] += 1;
        }

        spawnIn -= 1;
        if (spawnIn <= 0) {
          spawn();
          spawnIn = spawnInterval() + Math.floor(Math.random() * 20);
        }

        for (var i = packets.length - 1; i >= 0; i--) {
          var p = packets[i];
          p.y += fallSpeed();
          if (p.y < PORT_Y - 4) continue;
          packets.splice(i, 1);
          packetHandled();
          if (p.bad) {
            portFx[p.lane] = -14;
            K.burst(laneCenter(p.lane), PORT_Y - 4, "#ff6b6b", 10);
            loseLife();
          } else {
            portFx[p.lane] = 14;
            score += 5;
            K.beep(500, 0.05, 0.03);
          }
        }
      },
      key: function (code) {
        if (code === "ArrowLeft" || code === "KeyA") moveTo(lane - 1);
        else if (code === "ArrowRight" || code === "KeyD") moveTo(lane + 1);
        else if (code === "Space" || code === "ArrowUp") zap();
      },
      pointermove: function (x) {
        moveTo(Math.floor((x - X0) / LANE_W));
      },
      pointerdown: function () {
        zap();
      },
      draw: function () {
        var ctx = K.ctx;
        for (var li = 0; li < 3; li++) K.heart(6 + li * 10, 6, li < lives);

        // lane separators
        ctx.fillStyle = "#1c212c";
        for (var s = 0; s <= LANES; s++) {
          for (var y = K.TOP + 4; y < PORT_Y; y += 6) {
            ctx.fillRect(X0 + s * LANE_W, y, 1, 3);
          }
        }

        // ports: sockets that flash green on a good landing, red on a breach
        for (var l = 0; l < LANES; l++) {
          var cx = laneCenter(l);
          ctx.fillStyle = portFx[l] > 0 ? "#69db7c" : portFx[l] < 0 ? "#ff6b6b" : "#333b45";
          ctx.fillRect(cx - 14, PORT_Y, 28, 6);
          ctx.fillStyle = "#0d1017";
          ctx.fillRect(cx - 10, PORT_Y + 2, 8, 2);
          ctx.fillRect(cx + 2, PORT_Y + 2, 8, 2);
          K.drawText(PORTS[l], cx - K.textWidth(PORTS[l], 1) / 2, PORT_Y + 8, 1, "#4d5766");
        }

        // packets
        for (var i = 0; i < packets.length; i++) {
          var p = packets[i];
          var px = laneCenter(p.lane) - 5 + (Math.floor((tick + p.wob) / 10) % 2); // slight wobble
          var py = Math.floor(p.y);
          if (p.bad) {
            ctx.fillStyle = "#ff6b6b";
            ctx.fillRect(px, py, 10, 10);
            ctx.fillStyle = "#0d1017";
            ctx.fillRect(px + 2, py + 2, 2, 2);
            ctx.fillRect(px + 6, py + 2, 2, 2);
            ctx.fillRect(px + 4, py + 4, 2, 2);
            ctx.fillRect(px + 2, py + 6, 2, 2);
            ctx.fillRect(px + 6, py + 6, 2, 2);
            ctx.fillStyle = "#ff6b6b";
            ctx.fillRect(px - 2, py + 4, 2, 2); // spikes
            ctx.fillRect(px + 10, py + 4, 2, 2);
          } else {
            ctx.fillStyle = "#69db7c";
            ctx.fillRect(px, py, 10, 10);
            ctx.fillStyle = "#0d1017";
            ctx.fillRect(px + 4, py + 4, 2, 2);
          }
        }

        // zap beam
        if (zapFx) {
          var bx = laneCenter(zapFx.lane);
          ctx.fillStyle = zapFx.t % 2 === 0 ? "#ffd43b" : "#ffffff";
          for (var by = K.TOP + 4; by < PLAYER_Y; by += 4) {
            ctx.fillRect(bx - 1, by, 2, 3);
          }
        }

        // the daemon at the gate: red body, horns, white eyes
        var dx = laneCenter(lane) - 7;
        ctx.fillStyle = "#ff6b6b";
        ctx.fillRect(dx, PLAYER_Y, 14, 10);
        ctx.fillRect(dx + 1, PLAYER_Y - 3, 2, 3); // horns
        ctx.fillRect(dx + 11, PLAYER_Y - 3, 2, 3);
        ctx.fillStyle = "#ffffff";
        ctx.fillRect(dx + 3, PLAYER_Y + 3, 3, 3); // eyes
        ctx.fillRect(dx + 8, PLAYER_Y + 3, 3, 3);
        if (cool > 0) {
          ctx.fillStyle = "#ffd43b";
          ctx.fillRect(dx + 6, PLAYER_Y - 2, 2, 2); // recharging tell
        }

        if (tick < flashUntil) K.centerText("wave " + wave, 100, 3, "#ffd43b");
      },
      score: function () {
        return score;
      }
    };
  });
})();
