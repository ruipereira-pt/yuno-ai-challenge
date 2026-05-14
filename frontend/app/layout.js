import "./globals.css";

export const metadata = {
  title: "PSP Health Dashboard",
  description: "Live dashboard for PSP health, alerts, and ranking."
};

export default function RootLayout({ children }) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
