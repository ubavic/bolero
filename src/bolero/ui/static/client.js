(function () {
  var view = document.body.getAttribute("data-bolero-view");
  if (!view) {
    return;
  }
  var proto = location.protocol === "https:" ? "wss:" : "ws:";
  var ws = new WebSocket(proto + "//" + location.host + "/bolero/ws");

  ws.addEventListener("open", function () {
    ws.send(JSON.stringify({ event: "open", view: view }));
  });

  ws.addEventListener("message", function (ev) {
    var msg = JSON.parse(ev.data);
    if (!msg || !msg.replace) {
      return;
    }
    for (var i = 0; i < msg.replace.length; i++) {
      var part = msg.replace[i];
      var target = document.querySelector('[data-bolero-id="' + cssEscape(part.id) + '"]');
      if (target) {
        target.outerHTML = part.html;
      }
    }
  });

  function send(obj) {
    if (ws.readyState === 1) {
      ws.send(JSON.stringify(obj));
    }
  }

  document.addEventListener("change", function (ev) {
    var el = ev.target.closest("[data-bolero-send]");
    if (!el || el.getAttribute("data-bolero-event") !== "change") {
      return;
    }
    var value = el.value;
    if (el.type === "checkbox") {
      value = el.checked ? "true" : "false";
    }
    send({ id: el.getAttribute("data-bolero-send"), event: "change", value: value });
  });

  document.addEventListener("click", function (ev) {
    var el = ev.target.closest("[data-bolero-event]");
    if (!el) {
      return;
    }
    var event = el.getAttribute("data-bolero-event");
    if (event === "change") {
      return;
    }
    send({
      id: el.getAttribute("data-bolero-send"),
      event: event,
      value: el.getAttribute("data-bolero-value") || ""
    });
  });

  function cssEscape(s) {
    if (window.CSS && CSS.escape) {
      return CSS.escape(s);
    }
    return String(s).replace(/["\\]/g, "\\$&");
  }
})();
