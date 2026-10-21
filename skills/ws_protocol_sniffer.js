// skills/ws_protocol_sniffer.js
// Open-Source Sub-Agent Skill: Real-Time WebSocket & Socket.IO Protocol Sniffer
// Intercepts Socket.IO (42["event", payload]) and JSON WebSocket frames to
// classify match_found, incoming_sms, outgoing_sms, typing, and partner_disconnected.
(function () {
  if (window.__chitchatWSSnifferInstalled) return window.__chitchatWSFrames;
  window.__chitchatWSSnifferInstalled = true;
  var frames = (window.__chitchatWSFrames = []);

  function classifyFrame(dir, raw) {
    var text = String(raw || "");
    var s = text.toLowerCase();
    var kind = "unknown";
    if (/(matched|match_found|partner_found|connected|room_joined|start_chat)/i.test(s)) kind = "match_found";
    else if (/(receive-message|chat_message|new_message|stranger_msg|message|msg)/i.test(s)) kind = dir === "in" ? "incoming_sms" : "outgoing_sms";
    else if (/(typing|is_typing|partnertyping)/i.test(s)) kind = "typing";
    else if (/(disconnect|partner_left|stranger_left|chat_ended|leave|skip)/i.test(s)) kind = "chat_ended";
    return { ts: Date.now(), dir: dir, kind: kind, sample: text.slice(0, 500) };
  }

  var OrigWS = window.WebSocket;
  if (OrigWS) {
    var origSend = OrigWS.prototype.send;
    OrigWS.prototype.send = function (data) {
      try {
        frames.push(classifyFrame("out", data));
        if (frames.length > 300) frames.shift();
      } catch (_) {}
      return origSend.apply(this, arguments);
    };
    var PatchedWS = function (url, protocols) {
      var ws = protocols !== undefined ? new OrigWS(url, protocols) : new OrigWS(url);
      ws.addEventListener("message", function (ev) {
        try {
          frames.push(classifyFrame("in", ev.data));
          if (frames.length > 300) frames.shift();
        } catch (_) {}
      });
      return ws;
    };
    PatchedWS.prototype = OrigWS.prototype;
    window.WebSocket = PatchedWS;
  }
  return frames;
})();
