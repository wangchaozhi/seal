/**
 * 弱阻吓：不能视为真正安全边界。
 * 真正敏感的字体、算法、权限和高清文件必须放在服务端。
 */
export function installBrowserDeterrence(): () => void {
  const enabled = import.meta.env.PROD &&
    import.meta.env.VITE_ENABLE_BROWSER_DETERRENCE === "true";

  if (!enabled) return () => undefined;

  const onContextMenu = (event: MouseEvent) => {
    event.preventDefault();
  };

  const onKeyDown = (event: KeyboardEvent) => {
    const key = event.key.toLowerCase();
    const blocked =
      event.key === "F12" ||
      (event.ctrlKey && event.shiftKey && ["i", "j", "c"].includes(key)) ||
      (event.ctrlKey && key === "u");

    if (blocked) {
      event.preventDefault();
      window.dispatchEvent(new CustomEvent("security-warning"));
    }
  };

  document.addEventListener("contextmenu", onContextMenu);
  document.addEventListener("keydown", onKeyDown);

  return () => {
    document.removeEventListener("contextmenu", onContextMenu);
    document.removeEventListener("keydown", onKeyDown);
  };
}
