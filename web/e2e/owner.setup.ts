import { test as setup } from "@playwright/test";

import { ownerSessionFile } from "./venue";

// One login per run: the API allows five logins a minute from one address, and the owner tests
// reuse this session instead of logging in each time.
setup("log in as the demo owner", async ({ page }) => {
  await page.goto("/owner/login");
  await page.getByRole("button", { name: "Isi akun demo" }).click();
  await page.getByRole("button", { name: "Masuk", exact: true }).click();
  await page.waitForURL("**/owner");
  await page.context().storageState({ path: ownerSessionFile });
});
