import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import "./styles.css";
import "./hand.css";
import "./matchConnection.css";
import "./choices.css";

createRoot(document.getElementById("root")!).render(<StrictMode><App /></StrictMode>);
