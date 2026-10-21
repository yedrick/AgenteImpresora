<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Concerns\HasUuids;
use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\BelongsTo;

/**
 * Un ticket, comanda o factura pendiente de imprimir en un local.
 */
class CollatechTrabajo extends Model
{
    use HasUuids;

    public const PENDIENTE = 'pending';
    public const RECIBIDO = 'received';
    public const IMPRESO = 'printed';
    public const FALLIDO = 'failed';
    public const CADUCADO = 'expired';

    protected $table = 'collatech_trabajos';

    protected $fillable = ['agente_id', 'estacion', 'tipo', 'datos', 'estado', 'caduca_en'];

    protected function casts(): array
    {
        return [
            'datos' => 'array',
            'detalle' => 'array',
            'caduca_en' => 'datetime',
            'recibido_en' => 'datetime',
            'terminado_en' => 'datetime',
        ];
    }

    public function agente(): BelongsTo
    {
        return $this->belongsTo(CollatechAgente::class, 'agente_id');
    }

    public function terminado(): bool
    {
        return in_array($this->estado, [self::IMPRESO, self::FALLIDO, self::CADUCADO], true);
    }

    public function impreso(): bool
    {
        return $this->estado === self::IMPRESO;
    }
}
