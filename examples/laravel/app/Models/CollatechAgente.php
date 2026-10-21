<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\HasMany;
use Illuminate\Support\Str;

/**
 * La PC de un local con CollaTech Agent. Se sabe si esta encendida por el
 * latido que manda cada 30 segundos.
 */
class CollatechAgente extends Model
{
    protected $table = 'collatech_agentes';

    protected $fillable = ['nombre', 'token_hash'];

    protected $hidden = ['token_hash'];

    protected function casts(): array
    {
        return [
            'ultimo_contacto' => 'datetime',
            'encendido_desde' => 'datetime',
            'estaciones' => 'array',
        ];
    }

    /**
     * Segundos sin latido a partir de los cuales se da el agente por apagado.
     * El agente late cada 30: con 90 se toleran dos latidos perdidos.
     */
    public const SEGUNDOS_SIN_LATIDO = 90;

    public function trabajos(): HasMany
    {
        return $this->hasMany(CollatechTrabajo::class, 'agente_id');
    }

    /**
     * Crea un agente y devuelve su token. El token solo existe en este
     * momento: en la base de datos se guarda su hash.
     *
     * @return array{0: self, 1: string}
     */
    public static function crear(string $nombre): array
    {
        $token = Str::random(48);
        $agente = static::create(['nombre' => $nombre, 'token_hash' => hash('sha256', $token)]);

        return [$agente, $token];
    }

    public static function porToken(?string $token): ?self
    {
        if (! $token) {
            return null;
        }

        return static::where('token_hash', hash('sha256', $token))->first();
    }

    /** Si la PC del local esta encendida y con el agente funcionando. */
    public function enLinea(): bool
    {
        return $this->ultimo_contacto !== null
            && $this->ultimo_contacto->gt(now()->subSeconds(self::SEGUNDOS_SIN_LATIDO));
    }

    /**
     * El ultimo estado conocido de una estacion, tal como lo manda el agente:
     * name, online, ready y targets (con paper, cover_open, error...).
     */
    public function estacion(string $nombre): ?array
    {
        foreach ($this->estaciones ?? [] as $estacion) {
            if (strcasecmp($estacion['name'] ?? '', $nombre) === 0) {
                return $estacion;
            }
        }

        return null;
    }

    /**
     * Si se puede contar con esa impresora ahora mismo: agente encendido,
     * impresora respondiendo, con papel y la tapa cerrada.
     */
    public function puedeImprimir(string $estacion): bool
    {
        return $this->enLinea() && (bool) ($this->estacion($estacion)['ready'] ?? false);
    }

    /**
     * Por que no se puede imprimir, en palabras para el usuario. Null si todo
     * esta bien.
     */
    public function problema(string $estacion): ?string
    {
        if (! $this->enLinea()) {
            $desde = $this->ultimo_contacto?->diffForHumans() ?? 'nunca';

            return "La PC del local '{$this->nombre}' no responde (ultimo contacto: {$desde}).";
        }
        $est = $this->estacion($estacion);
        if ($est === null) {
            return "No hay ninguna estacion '{$estacion}' en el local '{$this->nombre}'.";
        }
        if ($est['ready'] ?? false) {
            return null;
        }
        foreach ($est['targets'] ?? [] as $t) {
            if (! ($t['ready'] ?? false)) {
                return "La impresora '{$estacion}': ".($t['error'] ?? 'no esta lista').'.';
            }
        }

        return "La impresora '{$estacion}' no esta lista.";
    }
}
