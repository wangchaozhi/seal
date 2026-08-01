import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import { installBrowserDeterrence } from "./security/browserDeterrence";

const uninstall = installBrowserDeterrence();
window.addEventListener("beforeunload", uninstall, { once: true });

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
