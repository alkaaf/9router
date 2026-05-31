import React from "react";
import { flushSync } from "react-dom";

function act(callback) {
  let result;
  flushSync(() => {
    result = callback();
  });
  return Promise.resolve(result);
}

// Set React.act so react-dom/test-utils can find it
Object.defineProperty(React, "act", {
  value: act,
  writable: true,
  configurable: true,
});
