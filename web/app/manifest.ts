import type { MetadataRoute } from "next";

// PWA manifest: the POS is installed on the tablet home screen and opens full-screen in landscape.
export default function manifest(): MetadataRoute.Manifest {
  return {
    name: "Mộc POS",
    short_name: "Mộc POS",
    description: "Bán hàng chuỗi cà phê Mộc",
    lang: "vi",
    start_url: "/pos",
    scope: "/",
    display: "standalone",
    orientation: "landscape",
    background_color: "#f5f7fa",
    theme_color: "#14213d",
    icons: [{ src: "/favicon.ico", sizes: "any", type: "image/x-icon" }],
  };
}
