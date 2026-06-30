import {
  ChangeDetectionStrategy,
  Component,
  inject,
  OnInit,
  signal,
} from "@angular/core";
import { HttpClient, HttpErrorResponse } from "@angular/common/http";
import { FormControl, FormGroup, Validators } from "@angular/forms";
import { AppModule } from "../../app.module";
import { DocumentTitleComponent } from "../../layout/document-title.component";
import { apiBase } from "../../../environments/environment";

interface ProwlarrConfig {
  enabled: boolean;
  baseUrl: string;
  apiKey: string;
  updatedAt: string;
}

@Component({
  selector: "app-prowlarr-settings",
  standalone: true,
  imports: [AppModule, DocumentTitleComponent],
  templateUrl: "./prowlarr-settings.component.html",
  styleUrl: "./prowlarr-settings.component.scss",
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class ProwlarrSettingsComponent implements OnInit {
  private http = inject(HttpClient);

  config = signal<ProwlarrConfig | null>(null);
  loading = signal(false);
  saving = signal(false);
  error = signal<string | null>(null);
  success = signal(false);

  /* eslint-disable @typescript-eslint/unbound-method */
  form = new FormGroup({
    enabled: new FormControl(false),
    baseUrl: new FormControl("", [Validators.pattern("https?://.+")]),
    apiKey: new FormControl(""),
  });
  /* eslint-enable @typescript-eslint/unbound-method */

  ngOnInit() {
    this.loadConfig();
  }

  loadConfig() {
    this.loading.set(true);
    this.error.set(null);
    this.http.get<ProwlarrConfig>(`${apiBase}/api/prowlarr`).subscribe({
      next: (cfg) => {
        this.config.set(cfg);
        this.form.patchValue({
          enabled: cfg.enabled,
          baseUrl: cfg.baseUrl,
          apiKey: cfg.apiKey,
        });
        this.loading.set(false);
      },
      error: (err: HttpErrorResponse) => {
        this.error.set(err.message ?? "Failed to load config");
        this.loading.set(false);
      },
    });
  }

  save() {
    if (this.form.invalid) return;
    this.saving.set(true);
    this.error.set(null);
    this.success.set(false);
    const { enabled, baseUrl, apiKey } = this.form.value;
    this.http
      .put<ProwlarrConfig>(`${apiBase}/api/prowlarr`, {
        enabled: enabled ?? false,
        baseUrl: baseUrl ?? "",
        apiKey: apiKey ?? "",
      })
      .subscribe({
        next: (cfg) => {
          this.config.set(cfg);
          this.saving.set(false);
          this.success.set(true);
        },
        error: (err: HttpErrorResponse) => {
          const body = err.error as { error?: string } | null;
          this.error.set(body?.error ?? err.message ?? "Failed to save config");
          this.saving.set(false);
        },
      });
  }
}
