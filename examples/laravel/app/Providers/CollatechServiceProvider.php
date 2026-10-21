<?php

namespace App\Providers;

use App\Console\Commands\CollatechAgenteCommand;
use Illuminate\Support\ServiceProvider;

/**
 * Registra las rutas que llama el agente y el comando de alta de locales.
 * Se activa anadiendolo en bootstrap/providers.php.
 */
class CollatechServiceProvider extends ServiceProvider
{
    public function boot(): void
    {
        $this->loadRoutesFrom(base_path('routes/collatech.php'));

        if ($this->app->runningInConsole()) {
            $this->commands([CollatechAgenteCommand::class]);
        }
    }
}
