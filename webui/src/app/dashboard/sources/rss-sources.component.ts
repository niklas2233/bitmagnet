import {
  ChangeDetectionStrategy,
  Component,
  inject,
  OnInit,
  signal,
} from "@angular/core";
import { HttpClient } from "@angular/common/http";
import { FormControl, FormGroup, Validators } from "@angular/forms";
import { AppModule } from "../../app.module";
import { DocumentTitleComponent } from "../../layout/document-title.component";
import { apiBase } from "../../../environments/environment";

interface RssFeed {
  id: string;
  url: string;
  source: string;
  createdAt: string;
}

@Component({
  selector: "app-rss-sources",
  standalone: true,
  imports: [AppModule, DocumentTitleComponent],
  templateUrl: "./rss-sources.component.html",
  styleUrl: "./rss-sources.component.scss",
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class RssSourcesComponent implements OnInit {
  private http = inject(HttpClient);

  feeds = signal<RssFeed[]>([]);
  loading = signal(false);
  error = signal<string | null>(null);
  adding = signal(false);

  displayedColumns = ["source", "url", "createdAt", "actions"];

  form = new FormGroup({
    url: new FormControl("", [Validators.required, Validators.pattern("https?://.+")]),
    source: new FormControl("", [Validators.required, Validators.pattern("[a-z0-9_-]+")]),
  });

  ngOnInit() {
    this.loadFeeds();
  }

  loadFeeds() {
    this.loading.set(true);
    this.error.set(null);
    this.http.get<RssFeed[]>(`${apiBase}/api/rss-feeds`).subscribe({
      next: (feeds) => {
        this.feeds.set(feeds);
        this.loading.set(false);
      },
      error: (err) => {
        this.error.set(err.message ?? "Failed to load feeds");
        this.loading.set(false);
      },
    });
  }

  addFeed() {
    if (this.form.invalid) return;
    this.adding.set(true);
    this.error.set(null);
    const { url, source } = this.form.value;
    this.http
      .post<RssFeed>(`${apiBase}/api/rss-feeds`, { url, source })
      .subscribe({
        next: (feed) => {
          this.feeds.update((feeds) => [...feeds, feed]);
          this.form.reset();
          this.adding.set(false);
        },
        error: (err) => {
          this.error.set(err.error?.error ?? err.message ?? "Failed to add feed");
          this.adding.set(false);
        },
      });
  }

  deleteFeed(source: string) {
    this.http
      .delete(`${apiBase}/api/rss-feeds/${encodeURIComponent(source)}`)
      .subscribe({
        next: () => {
          this.feeds.update((feeds) => feeds.filter((f) => f.source !== source));
        },
        error: (err) => {
          this.error.set(err.error?.error ?? err.message ?? "Failed to delete feed");
        },
      });
  }
}
