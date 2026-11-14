import { cleanup } from "@testing-library/react";
import { afterEach } from "vitest";

import { installFakeApi, resetFakeApi } from "./fakeApi";

installFakeApi();

// jsdom has no modal dialog support; opening and closing is all the booking sheet needs.
HTMLDialogElement.prototype.showModal = function showModal() {
  this.setAttribute("open", "");
};
HTMLDialogElement.prototype.close = function close() {
  this.removeAttribute("open");
  this.dispatchEvent(new Event("close"));
};

afterEach(() => {
  cleanup();
  resetFakeApi();
});
